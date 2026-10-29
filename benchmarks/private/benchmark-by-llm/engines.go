// Package authzbench compares Casbin, OPA and Speedle+ on the same policies.
package authzbench

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/casbin/casbin/v3"
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

func (s *SpeedleEngine) Name() string { return "speedle" }

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
