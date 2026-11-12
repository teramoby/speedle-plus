// Package authzbench compares Casbin, OPA, Cedar and Speedle+ on the same policies.
package authzbench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/casbin/casbin/v3"
	"github.com/cedar-policy/cedar-go"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"github.com/open-policy-agent/opa/v1/util"
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
)

// Req is one authorization question, shared by all engines.
type Req struct {
	User     string
	Groups   []string
	Tenant   string
	Action   string
	Resource string
	Attrs    map[string]any
}

// Engine answers authorization questions.
type Engine interface {
	Name() string
	Allowed(r Req) (bool, error)
	// Prepare converts r to the engine's native request once, so benchmarks
	// time only the decision itself.
	Prepare(r Req) func() (bool, error)
}

// ---------------------------------------------------------------- Casbin

// CasbinAttrs carries ABAC attributes; Casbin matchers read struct fields.
type CasbinAttrs struct {
	Amount float64
	Att1   string
	Att2   string
}

// CasbinObj is a resource with an owner, for r.obj.Owner matchers.
type CasbinObj struct {
	Name  string
	Owner string
}

type enforcer interface {
	Enforce(rvals ...interface{}) (bool, error)
}

type CasbinEngine struct {
	E      enforcer
	ToArgs func(Req) []any
}

func (c *CasbinEngine) Name() string { return "casbin" }

func (c *CasbinEngine) Allowed(r Req) (bool, error) { return c.E.Enforce(c.ToArgs(r)...) }

func (c *CasbinEngine) Prepare(r Req) func() (bool, error) {
	args := c.ToArgs(r)
	return func() (bool, error) { return c.E.Enforce(args...) }
}

// CasbinArgs is the default request mapping: r = sub, obj, act.
func CasbinArgs(r Req) []any { return []any{r.User, r.Resource, r.Action} }

// CasbinAttrArgs maps to r = sub, obj, act, attrs.
func CasbinAttrArgs(r Req) []any {
	a := CasbinAttrs{}
	if v, ok := r.Attrs["amount"].(float64); ok {
		a.Amount = v
	}
	a.Att1, _ = r.Attrs["att1"].(string)
	a.Att2, _ = r.Attrs["att2"].(string)
	return []any{r.User, r.Resource, r.Action, a}
}

func NewCasbin(modelPath, policyPath string, toArgs func(Req) []any) (*CasbinEngine, error) {
	e, err := casbin.NewEnforcer(modelPath, policyPath)
	if err != nil {
		return nil, err
	}
	return &CasbinEngine{E: e, ToArgs: toArgs}, nil
}

// NewCasbinSynced uses SyncedEnforcer, Casbin's goroutine-safe enforcer.
func NewCasbinSynced(modelPath, policyPath string, toArgs func(Req) []any) (*CasbinEngine, error) {
	e, err := casbin.NewSyncedEnforcer(modelPath, policyPath)
	if err != nil {
		return nil, err
	}
	return &CasbinEngine{E: e, ToArgs: toArgs}, nil
}

// ---------------------------------------------------------------- OPA

type OPAEngine struct {
	Q       rego.PreparedEvalQuery
	Store   storage.Store
	ToInput func(Req) map[string]any
}

func (o *OPAEngine) Name() string { return "opa" }

func (o *OPAEngine) Allowed(r Req) (bool, error) { return o.Prepare(r)() }

func (o *OPAEngine) Prepare(r Req) func() (bool, error) {
	input := o.ToInput(r)
	ctx := context.Background()
	return func() (bool, error) {
		rs, err := o.Q.Eval(ctx, rego.EvalInput(input))
		if err != nil {
			return false, err
		}
		return rs.Allowed(), nil
	}
}

// OPAInput is the input document every scenario policy reads.
func OPAInput(r Req) map[string]any {
	groups := make([]any, 0, len(r.Groups))
	for _, g := range r.Groups {
		groups = append(groups, g)
	}
	in := map[string]any{"user": r.User, "groups": groups, "action": r.Action, "resource": r.Resource}
	if r.Tenant != "" {
		in["tenant"] = r.Tenant
	}
	if r.Attrs != nil {
		in["attrs"] = r.Attrs
	}
	return in
}

// NewOPA compiles regoPath and loads dataPath (JSON) into an in-memory store.
// The query is always data.authz.allow.
func NewOPA(regoPath, dataPath string) (*OPAEngine, error) {
	mod, err := os.ReadFile(regoPath)
	if err != nil {
		return nil, err
	}
	data := map[string]any{}
	if dataPath != "" {
		b, err := os.ReadFile(dataPath)
		if err != nil {
			return nil, err
		}
		if err := util.UnmarshalJSON(b, &data); err != nil {
			return nil, fmt.Errorf("%s: %w", dataPath, err)
		}
	}
	store := inmem.NewFromObject(data)
	q, err := rego.New(
		rego.Query("data.authz.allow"),
		rego.Module(filepath.Base(regoPath), string(mod)),
		rego.Store(store),
	).PrepareForEval(context.Background())
	if err != nil {
		return nil, err
	}
	return &OPAEngine{Q: q, Store: store, ToInput: OPAInput}, nil
}

// ---------------------------------------------------------------- Speedle+

type SpeedleEngine struct {
	Ev ads.PolicyEvaluator
	// Map rewrites a request before evaluation, e.g. to put the tenant in
	// the resource path. Nil means no rewrite.
	Map func(Req) Req
}

func (s *SpeedleEngine) Name() string { return SpeedleLabel() }

// SpeedleLabel names the Speedle+ build under test. The harness is built
// once per Speedle+ version (go.mod: current master; go.may.mod: the commit
// that was master throughout May 2026, 8fb1481d, unchanged from 2026-01-16
// until 2026-06-24), and each build labels its results through SPEEDLE_LABEL.
func SpeedleLabel() string {
	if label := os.Getenv("SPEEDLE_LABEL"); label != "" {
		return label
	}
	return "speedle"
}

func (s *SpeedleEngine) Allowed(r Req) (bool, error) { return s.Prepare(r)() }

func (s *SpeedleEngine) Prepare(r Req) func() (bool, error) {
	rc := s.Ctx(r)
	return func() (bool, error) {
		ok, _, err := s.Ev.IsAllowed(rc)
		return ok, err
	}
}

// Ctx builds the Speedle request; all scenario files use [service.app].
func (s *SpeedleEngine) Ctx(r Req) ads.RequestContext {
	if s.Map != nil {
		r = s.Map(r)
	}
	ps := []*ads.Principal{{Type: ads.PRINCIPAL_TYPE_USER, Name: r.User}}
	for _, g := range r.Groups {
		ps = append(ps, &ads.Principal{Type: ads.PRINCIPAL_TYPE_GROUP, Name: g})
	}
	return ads.RequestContext{
		Subject:     &ads.Subject{Principals: ps},
		ServiceName: "app",
		Resource:    r.Resource,
		Action:      r.Action,
		Attributes:  r.Attrs,
	}
}

func NewSpeedle(spdlPath string, watch bool) (*SpeedleEngine, error) {
	ev, err := eval.NewFromFile(spdlPath, watch)
	if err != nil {
		return nil, err
	}
	return &SpeedleEngine{Ev: ev}, nil
}

// ---------------------------------------------------------------- Cedar

// CedarRef is a Cedar entity reference.
type CedarRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// CedarReq is one request in Cedar's terms. It is also written as JSON for
// the Rust harness (cmd/cedar-rs), so both Cedar implementations answer
// exactly the same questions. Context values are strings, int64, bool or
// CedarRef. PrincipalParents are added to the principal entity for this
// request only: request-supplied groups, as Cedar callers pass them in the
// entity slice of a request.
type CedarReq struct {
	Principal        CedarRef       `json:"principal"`
	Action           CedarRef       `json:"action"`
	Resource         CedarRef       `json:"resource"`
	Context          map[string]any `json:"context"`
	PrincipalParents []CedarRef     `json:"principal_parents,omitempty"`
}

// CedarDefault maps a request: User, Action and Resource entities named after
// the request; the resource path and the attributes go to the context.
func CedarDefault(r Req) CedarReq {
	c := CedarReq{
		Principal: CedarRef{"User", r.User},
		Action:    CedarRef{"Action", r.Action},
		Resource:  CedarRef{"Resource", r.Resource},
		Context:   map[string]any{"path": r.Resource},
	}
	for k, v := range r.Attrs {
		// Cedar numbers are 64-bit integers; every scenario amount is integral.
		if f, ok := v.(float64); ok && f == float64(int64(f)) {
			v = int64(f)
		}
		c.Context[k] = v
	}
	for _, g := range r.Groups {
		c.PrincipalParents = append(c.PrincipalParents, CedarRef{"Group", g})
	}
	return c
}

func (r CedarRef) uid() cedar.EntityUID {
	return cedar.NewEntityUID(cedar.EntityType(r.Type), cedar.String(r.ID))
}

func cedarValue(v any) (cedar.Value, error) {
	switch x := v.(type) {
	case string:
		return cedar.String(x), nil
	case int64:
		return cedar.Long(x), nil
	case int:
		return cedar.Long(x), nil
	case bool:
		return cedar.Boolean(x), nil
	case CedarRef:
		return x.uid(), nil
	}
	return nil, fmt.Errorf("unsupported Cedar context value %T", v)
}

// cedarOverlay adds one request's principal entity to the shared entities.
type cedarOverlay struct {
	base  cedar.EntityGetter
	extra cedar.Entity
}

func (o cedarOverlay) Get(uid cedar.EntityUID) (cedar.Entity, bool) {
	if uid == o.extra.UID {
		return o.extra, true
	}
	return o.base.Get(uid)
}

// CedarEngine evaluates with cedar-go, the Cedar project's Go implementation.
type CedarEngine struct {
	Label    string
	Policies *cedar.PolicySet
	Entities cedar.EntityMap
	Map      func(Req) CedarReq
}

func (c *CedarEngine) Name() string { return c.Label }

func (c *CedarEngine) Allowed(r Req) (bool, error) { return c.Prepare(r)() }

// Prepare builds the native request (and any request-specific entity)
// outside the timed decision, as for the other engines.
func (c *CedarEngine) Prepare(r Req) func() (bool, error) {
	m := c.Map
	if m == nil {
		m = CedarDefault
	}
	cr := m(r)
	record := cedar.RecordMap{}
	for k, v := range cr.Context {
		value, err := cedarValue(v)
		if err != nil {
			return func() (bool, error) { return false, err }
		}
		record[cedar.String(k)] = value
	}
	req := cedar.Request{Principal: cr.Principal.uid(), Action: cr.Action.uid(), Resource: cr.Resource.uid(),
		Context: cedar.NewRecord(record)}
	var entities cedar.EntityGetter = c.Entities
	if len(cr.PrincipalParents) > 0 {
		principal, _ := c.Entities.Get(req.Principal)
		principal.UID = req.Principal
		parents := principal.Parents.Slice()
		for _, p := range cr.PrincipalParents {
			parents = append(parents, p.uid())
		}
		principal.Parents = cedar.NewEntityUIDSet(parents...)
		entities = cedarOverlay{base: c.Entities, extra: principal}
	}
	return func() (bool, error) {
		decision, diag := cedar.Authorize(c.Policies, entities, req)
		if len(diag.Errors) > 0 {
			return false, fmt.Errorf("cedar policy %s: %s", diag.Errors[0].PolicyID, diag.Errors[0].Message)
		}
		return bool(decision), nil
	}
}

// NewCedar loads Cedar policies and entities (Cedar's JSON entity format).
func NewCedar(label, policyPath, entitiesPath string) (*CedarEngine, error) {
	src, err := os.ReadFile(policyPath)
	if err != nil {
		return nil, err
	}
	ps, err := cedar.NewPolicySetFromBytes(filepath.Base(policyPath), src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", policyPath, err)
	}
	entities := cedar.EntityMap{}
	if entitiesPath != "" {
		b, err := os.ReadFile(entitiesPath)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &entities); err != nil {
			return nil, fmt.Errorf("%s: %w", entitiesPath, err)
		}
	}
	return &CedarEngine{Label: label, Policies: ps, Entities: entities}, nil
}
