// Package playground adapts the pinned, unmodified Speedle+ parser and evaluator
// to an isolated, browser-only single-service playground.
package playground

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/eval"
	"github.com/teramoby/speedle-plus/pkg/eval/function"
	"github.com/teramoby/speedle-plus/pkg/pdl"
)

const Revision = "5cccf4c4e50a2b0ab901a9fa65acc39074ed1068"
const MaxInputBytes = 256 * 1024
const MaxStatements = 500

var builtinFunctions = map[string]govaluate.ExpressionFunction{
	"Sqrt": function.Sqrt, "Max": function.Max, "Min": function.Min,
	"Sum": function.Sum, "Avg": function.Avg, "IsSubSet": function.IsSubSet,
}
var evaluationMu sync.Mutex
var attributeName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,254}$`)

type Input struct {
	Policies string          `json:"policies"`
	Request  json.RawMessage `json:"request"`
}
type Error struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
	Line    int    `json:"line,omitempty"`
}
type Statement struct {
	Line   int    `json:"line"`
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
}
type Result struct {
	OK              bool                       `json:"ok"`
	Allowed         *bool                      `json:"allowed,omitempty"`
	Reason          string                     `json:"reason,omitempty"`
	ReasonCode      *int32                     `json:"reasonCode,omitempty"`
	Error           *Error                     `json:"error,omitempty"`
	Roles           []string                   `json:"roles,omitempty"`
	Policies        []*ads.EvaluatedPolicy     `json:"policies,omitempty"`
	RolePolicies    []*ads.EvaluatedRolePolicy `json:"rolePolicies,omitempty"`
	Statements      []Statement                `json:"statements,omitempty"`
	Attributes      map[string]interface{}     `json:"attributes,omitempty"`
	PolicyCount     int                        `json:"policyCount"`
	RolePolicyCount int                        `json:"rolePolicyCount"`
	DurationMs      float64                    `json:"durationMs"`
	Engine          string                     `json:"engine"`
	Revision        string                     `json:"revision"`
}
type jsonRequest struct {
	Subject     *ads.Subject    `json:"subject"`
	ServiceName string          `json:"serviceName"`
	Resource    string          `json:"resource"`
	Action      string          `json:"action"`
	Attributes  json.RawMessage `json:"attributes"`
}
type typedAttribute struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type logCapture struct{ messages []string }

func (h *logCapture) Levels() []log.Level {
	return []log.Level{log.ErrorLevel, log.WarnLevel, log.FatalLevel, log.PanicLevel}
}
func (h *logCapture) Fire(e *log.Entry) error { h.messages = append(h.messages, e.Message); return nil }

func decodeStrict(data []byte, target interface{}) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected one JSON object")
	}
	return nil
}
func fail(stage, message string, line int) Result { return Result{Error: &Error{stage, message, line}} }

// EvaluateJSON runs the genuine Speedle+ parser, policy cache, role resolver,
// condition engine and deny-overrides combiner. Wrapper failures omit allowed.
func EvaluateJSON(data []byte) (out Result) {
	evaluationMu.Lock()
	defer evaluationMu.Unlock()
	start := time.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			out = fail("evaluation", fmt.Sprintf("Engine could not evaluate input: %v", recovered), 0)
		}
		out.DurationMs = float64(time.Since(start).Microseconds()) / 1000
		out.Engine = "Speedle+ · Go WebAssembly"
		out.Revision = Revision
	}()
	if len(data) > MaxInputBytes {
		return fail("input", "Input exceeds the playground limit of 256 KiB.", 0)
	}
	var input Input
	if err := decodeStrict(data, &input); err != nil {
		return fail("input", err.Error(), 0)
	}
	if len(input.Request) == 0 || string(input.Request) == "null" {
		return fail("request", "Authorization request must be a JSON object.", 0)
	}
	ctx, err := parseRequest(input.Request)
	if err != nil {
		return fail("request", err.Error(), 0)
	}
	service := &pms.Service{Name: ctx.ServiceName, Type: pms.TypeApplication}
	statements := []Statement{}
	compiled := map[string]*govaluate.EvaluableExpression{}
	for index, rawLine := range strings.Split(strings.ReplaceAll(input.Policies, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if len(statements) >= MaxStatements {
			return fail("policy", "The playground supports at most 500 statements.", index+1)
		}
		name := fmt.Sprintf("Line %d", index+1)
		id := fmt.Sprintf("line-%d", index+1)
		var condition string
		var expressions []string
		var kind string
		// Upstream parsers expose distinct policy and role-policy entry points.
		// Sentinel validation rejects the trailing text those parsers otherwise ignore.
		role, roleErr := parseRoleStrict(line, name)
		if roleErr == nil {
			role.ID = id
			service.RolePolicies = append(service.RolePolicies, role)
			condition = role.Condition
			expressions = role.ResourceExpressions
			kind = "rolePolicy"
		} else {
			policy, policyErr := parsePolicyStrict(line, name)
			if policyErr != nil {
				return fail("policy", fmt.Sprintf("Invalid SPDL statement. Policy: %v; role policy: %v", policyErr, roleErr), index+1)
			}
			policy.ID = id
			service.Policies = append(service.Policies, policy)
			condition = policy.Condition
			kind = "policy"
			for _, permission := range policy.Permissions {
				if permission.ResourceExpression != "" {
					expressions = append(expressions, permission.ResourceExpression)
				}
			}
		}
		for _, pattern := range expressions {
			if len(pattern) > 1024 {
				return fail("policy", "Resource expression exceeds Speedle+'s 1024-byte regex limit.", index+1)
			}
			if _, err := regexp.Compile(pattern); err != nil {
				return fail("policy", "Invalid resource expression: "+err.Error(), index+1)
			}
		}
		if condition != "" {
			if len(condition) > 4096 {
				return fail("condition", "Condition exceeds Speedle+'s 4096-byte limit.", index+1)
			}
			expression, err := govaluate.NewEvaluableExpressionWithFunctions(condition, builtinFunctions)
			if err != nil {
				return fail("condition", err.Error()+" (Only Speedle built-in functions are available in this browser playground.)", index+1)
			}
			compiled[id] = expression
		}
		statements = append(statements, Statement{index + 1, id, kind, line})
	}
	if len(statements) == 0 {
		return fail("policy", "Enter at least one SPDL statement.", 0)
	}
	capture := &logCapture{}
	logger := log.StandardLogger()
	oldHooks := logger.ReplaceHooks(log.LevelHooks{})
	oldOut := logger.Out
	oldLevel := logger.Level
	logger.SetOutput(io.Discard)
	logger.SetLevel(log.WarnLevel)
	logger.AddHook(capture)
	defer func() { logger.ReplaceHooks(oldHooks); logger.SetOutput(oldOut); logger.SetLevel(oldLevel) }()
	store := eval.NewRuntimePolicyStore()
	store.Functions = builtinFunctions
	evaluator := &eval.PolicyEvalImpl{RuntimePolicyStore: store}
	if ctx.ServiceName != pms.GlobalService {
		evaluator.AddServiceInRuntimeCache(&pms.Service{Name: pms.GlobalService, Type: pms.TypeApplication})
	}
	evaluator.AddServiceInRuntimeCache(service)
	diagnostic, err := evaluator.Diagnose(*ctx)
	if err != nil {
		return fail("evaluation", err.Error(), 0)
	}
	if diagnostic == nil || diagnostic.Reason == ads.ERROR_IN_EVALUATION {
		return fail("evaluation", "Speedle reported ERROR_IN_EVALUATION.", 0)
	}
	// Upstream intentionally fail-closes some condition errors. In an editor those
	// are input errors, not an ordinary false; expose its exact diagnostic instead.
	if len(capture.messages) > 0 {
		return fail("evaluation", strings.Join(capture.messages, "\n"), 0)
	}
	// Enforce the documented boolean-condition contract for conditions actually
	// evaluated; upstream treats a non-bool result as a nonmatching condition.
	checked := map[string]bool{}
	for _, policy := range diagnostic.Policies {
		checked[policy.ID] = true
	}
	for _, policy := range diagnostic.RolePolicies {
		checked[policy.ID] = true
	}
	for id := range checked {
		if expression := compiled[id]; expression != nil {
			value, err := expression.Evaluate(diagnostic.Attributes)
			if err != nil {
				return fail("evaluation", err.Error(), lineFor(id, statements))
			}
			if _, ok := value.(bool); !ok {
				return fail("condition", "Condition must evaluate to a boolean, not a number, string or array.", lineFor(id, statements))
			}
		}
	}
	// Stable UI diagnostics; upstream collects relevant policies through maps.
	sort.Slice(diagnostic.Policies, func(i, j int) bool {
		return lineFor(diagnostic.Policies[i].ID, statements) < lineFor(diagnostic.Policies[j].ID, statements)
	})
	sort.Slice(diagnostic.RolePolicies, func(i, j int) bool {
		return lineFor(diagnostic.RolePolicies[i].ID, statements) < lineFor(diagnostic.RolePolicies[j].ID, statements)
	})
	sort.Strings(diagnostic.GrantedRoles)
	reasonCode := int32(diagnostic.Reason)
	return Result{OK: true, Allowed: &diagnostic.Allowed, Reason: diagnostic.Reason.String(), ReasonCode: &reasonCode, Roles: diagnostic.GrantedRoles, Policies: diagnostic.Policies, RolePolicies: diagnostic.RolePolicies, Statements: statements, Attributes: diagnostic.Attributes, PolicyCount: len(service.Policies), RolePolicyCount: len(service.RolePolicies)}
}
func lineFor(id string, statements []Statement) int {
	for _, s := range statements {
		if s.ID == id {
			return s.Line
		}
	}
	return 0
}
func parsePolicyStrict(line, name string) (*pms.Policy, error) {
	p, err := pdl.ParsePolicy(line, name)
	if err != nil {
		return nil, err
	}
	if p.Condition == "" {
		check, err := pdl.ParsePolicy(line+" if true", name)
		if err != nil || check.Condition != "true" {
			return nil, fmt.Errorf("unexpected trailing text or incomplete condition")
		}
	}
	return p, nil
}
func parseRoleStrict(line, name string) (*pms.RolePolicy, error) {
	p, err := pdl.ParseRolePolicy(line, name)
	if err != nil {
		return nil, err
	}
	if p.Condition == "" {
		check, err := pdl.ParseRolePolicy(line+" if true", name)
		if err != nil || check.Condition != "true" {
			return nil, fmt.Errorf("unexpected trailing text or incomplete condition")
		}
	}
	return p, nil
}
func parseRequest(data []byte) (*ads.RequestContext, error) {
	var request jsonRequest
	if err := decodeStrict(data, &request); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Action) == "" || strings.TrimSpace(request.Resource) == "" {
		return nil, fmt.Errorf("request.action and request.resource must be non-empty strings")
	}
	if request.ServiceName == "" {
		request.ServiceName = "playground"
	}
	if request.Subject != nil {
		if request.Subject.Token != "" || request.Subject.TokenType != "" || request.Subject.Asserted {
			return nil, fmt.Errorf("token assertion is not available in the browser; supply subject.principals instead")
		}
		if len(request.Subject.Principals) > 200 {
			return nil, fmt.Errorf("at most 200 request principals are supported")
		}
		for _, principal := range request.Subject.Principals {
			if principal == nil || principal.Name == "" {
				return nil, fmt.Errorf("every principal requires a non-empty name and type")
			}
			switch principal.Type {
			case "user", "group", "entity":
			default:
				return nil, fmt.Errorf("principal type %q is not supported: use user, group or entity; assign roles with SPDL role policies", principal.Type)
			}
		}
	}
	attributes := map[string]interface{}{}
	if len(request.Attributes) > 0 && string(request.Attributes) != "null" {
		var list []typedAttribute
		if err := decodeStrict(request.Attributes, &list); err != nil {
			return nil, fmt.Errorf("attributes must be an array of {name,type,value}: %v", err)
		}
		for _, attribute := range list {
			if !attributeName.MatchString(attribute.Name) {
				return nil, fmt.Errorf("invalid attribute name %q: use letters, digits and underscores, starting with a letter (max 255)", attribute.Name)
			}
			if strings.HasPrefix(attribute.Name, "request_") {
				return nil, fmt.Errorf("%q is a reserved runtime attribute; do not supply request_ attributes", attribute.Name)
			}
			if _, exists := attributes[attribute.Name]; exists {
				return nil, fmt.Errorf("duplicate attribute %q", attribute.Name)
			}
			value, err := convertValue(attribute.Type, attribute.Value)
			if err != nil {
				return nil, fmt.Errorf("attribute %q: %v", attribute.Name, err)
			}
			attributes[attribute.Name] = value
		}
	}
	return &ads.RequestContext{Subject: request.Subject, ServiceName: request.ServiceName, Resource: request.Resource, Action: request.Action, Attributes: attributes}, nil
}
func convertValue(kind string, value interface{}) (interface{}, error) {
	// Validate the declared type even when an array contains no elements.
	switch kind {
	case "string", "numeric", "bool", "datetime":
	default:
		return nil, fmt.Errorf("type must be string, numeric, bool or datetime")
	}
	if value == nil {
		return nil, fmt.Errorf("value must not be null")
	}
	if values, ok := value.([]interface{}); ok {
		converted := make([]interface{}, len(values))
		for i, v := range values {
			if _, nested := v.([]interface{}); nested {
				return nil, fmt.Errorf("nested arrays are not supported")
			}
			out, err := convertValue(kind, v)
			if err != nil {
				return nil, err
			}
			converted[i] = out
		}
		return converted, nil
	}
	switch kind {
	case "string":
		if _, ok := value.(string); ok {
			return value, nil
		}
	case "numeric":
		if _, ok := value.(float64); ok {
			return value, nil
		}
	case "bool":
		if _, ok := value.(bool); ok {
			return value, nil
		}
	case "datetime":
		if text, ok := value.(string); ok {
			for _, layout := range []string{time.RFC3339Nano, time.RubyDate, time.UnixDate} {
				if date, err := time.Parse(layout, text); err == nil {
					return float64(date.Unix()), nil
				}
			}
			return nil, fmt.Errorf("datetime must be RFC3339, RubyDate or UnixDate")
		}
	}
	return nil, fmt.Errorf("value does not match declared type %q", kind)
}
