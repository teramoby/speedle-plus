package eval

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
	ads "github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

func recoveryGrant(id string) *pms.Policy {
	p := featurePolicy(id, "")
	p.Principals = [][]string{{"user:alice"}}
	return p
}

func requireEvaluationError(t *testing.T, err error) {
	t.Helper()
	if errors.Code(err) != errors.EvalEngineError {
		t.Fatalf("expected evaluation error, got %v", err)
	}
}

func requireRecoveredDecision(t *testing.T, e *PolicyEvalImpl, ctx ads.RequestContext) {
	t.Helper()
	allowed, reason, err := e.IsAllowed(ctx)
	if allowed || reason != ads.ERROR_IN_EVALUATION {
		t.Fatalf("decision allowed=%v reason=%v err=%v", allowed, reason, err)
	}
	requireEvaluationError(t, err)
	diagnostic, err := e.Diagnose(ctx)
	requireEvaluationError(t, err)
	if diagnostic.Allowed || diagnostic.Reason != ads.ERROR_IN_EVALUATION || len(diagnostic.GrantedRoles)+len(diagnostic.Policies)+len(diagnostic.RolePolicies) != 0 {
		t.Fatalf("partial authorization escaped: %+v", diagnostic)
	}
	permissions, err := e.GetAllGrantedPermissions(ctx)
	requireEvaluationError(t, err)
	if len(permissions) != 0 {
		t.Fatalf("partial permissions escaped: %v", permissions)
	}
}

func requireHealthyAfterRecovery(t *testing.T, e *PolicyEvalImpl) {
	t.Helper()
	// A write must also succeed: a leaked read lock would allow more reads,
	// while deadlocking a writer after recovery.
	done := make(chan struct{})
	go func() {
		e.AddServiceInRuntimeCache(&pms.Service{Name: "app", Policies: []*pms.Policy{recoveryGrant("healthy")}})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("recovery leaked an evaluator lock")
	}
	if allowed, _, err := e.IsAllowed(featureContext()); !allowed || err != nil {
		t.Fatalf("healthy request after recovery: %v %v", allowed, err)
	}
}

func TestFeatureEvaluationPanicFailClosed(t *testing.T) {
	oldHooks := log.StandardLogger().ReplaceHooks(make(log.LevelHooks))
	hook := logtest.NewGlobal()
	t.Cleanup(func() { log.StandardLogger().ReplaceHooks(oldHooks) })
	for _, effect := range []string{pms.Grant, pms.Deny} {
		t.Run(effect, func(t *testing.T) {
			broken := recoveryGrant("broken")
			broken.Effect, broken.Condition = effect, "Crash(x)"
			e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("fallback"), broken}}}})
			expression, err := govaluate.NewEvaluableExpressionWithFunctions("Crash(x)", map[string]govaluate.ExpressionFunction{
				"Crash": func(...interface{}) (interface{}, error) { panic("secret-panic-payload") },
			})
			if err != nil {
				t.Fatal(err)
			}
			e.RuntimePolicyStore.RuntimeServices["app"].PoliciesCache.Conditions["broken"] = expression
			ctx := featureContext()
			ctx.Attributes = map[string]interface{}{"x": 1.0}
			requireRecoveredDecision(t, e, ctx)
			requireHealthyAfterRecovery(t, e)
		})
	}
	found := false
	for _, entry := range hook.AllEntries() {
		if entry.Message == "authorization engine recovered a panic" {
			found = true
			if entry.Level != log.ErrorLevel || entry.Data["stack"] == "" {
				t.Fatalf("missing error/stack: %+v", entry)
			}
		}
		if strings.Contains(fmt.Sprint(entry.Data), "secret-panic-payload") {
			t.Fatal("panic payload leaked into logs")
		}
	}
	if !found {
		t.Fatal("panic was not logged")
	}
}

func TestFeatureAsserterPanicRecovery(t *testing.T) {
	e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("grant")}}}})
	e.SetAsserterFunc(func(ctx *ads.RequestContext) error {
		ctx.Subject.Asserted = true
		ctx.Subject.Token = ""
		ctx.Subject.Principals[0].Name = "administrator"
		panic("assertion failed")
	})
	ctx := featureContext()
	ctx.Subject.TokenType, ctx.Subject.Token = "test", "token"
	requireRecoveredDecision(t, e, ctx)
	if ctx.Subject.Asserted || ctx.Subject.Token != "token" || ctx.Subject.Principals[0].Name != "alice" {
		t.Fatal("partial identity escaped recovery")
	}
	roles, err := e.GetAllGrantedRoles(ctx)
	requireEvaluationError(t, err)
	if len(roles) != 0 {
		t.Fatalf("partial roles: %v", roles)
	}
	e.SetAsserterFunc(nil)
	requireHealthyAfterRecovery(t, e)
}

func TestFeatureMalformedRequestRecovery(t *testing.T) {
	e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("grant")}}}})
	ctx := featureContext()
	ctx.Subject.Principals = append(ctx.Subject.Principals, nil)
	requireRecoveredDecision(t, e, ctx)
	if roles, err := e.GetAllGrantedRoles(ctx); len(roles) != 0 || errors.Code(err) != errors.EvalEngineError {
		t.Fatalf("roles=%v error=%v", roles, err)
	}
	requireHealthyAfterRecovery(t, e)
	// The exported pointer entry point must also contain a nil context panic.
	if allowed, reason, err := e.InternalIsAllowed(nil, nil); allowed || reason != ads.ERROR_IN_EVALUATION || errors.Code(err) != errors.EvalEngineError {
		t.Fatalf("nil context: %v %v %v", allowed, reason, err)
	}
}

func TestFeatureRoleStackExhaustionPrevention(t *testing.T) {
	if os.Getenv("SPEEDLE_RECOVERY_CHILD") != "1" {
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "-test.run=^TestFeatureRoleStackExhaustionPrevention$", "-test.timeout=40s")
		cmd.Env = append(os.Environ(), "SPEEDLE_RECOVERY_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bounded-stack process: %v\n%s", err, output)
		}
		return
	}
	previous := debug.SetMaxStack(1 << 20)
	defer debug.SetMaxStack(previous)
	// Exercise deep acyclic traversal, in both directions, without Go recursion.
	const size = 20000
	roles := make(map[string]*Role, size)
	for i := 0; i < size; i++ {
		name := fmt.Sprint(i)
		roles[name] = &Role{Name: name, ParentRoles: map[string]bool{}, ChildRoles: map[string]bool{}}
		if i > 0 {
			roles[name].ParentRoles[fmt.Sprint(i-1)] = true
			roles[fmt.Sprint(i-1)].ChildRoles[name] = true
		}
	}
	roles["0"].ParentPrincipals = map[string]bool{"user:alice": true}
	if denied, err := selfOrAncestorsBeDenied(fmt.Sprint(size-1), roles, nil); denied || err != nil {
		t.Fatalf("deep rooted graph: %v %v", denied, err)
	}
	if got := getDeniableDescendantRoles("0", roles); len(got) != size-1 {
		t.Fatalf("deep descendants: %d", len(got))
	}
	// Robustness against the original malformed, subjectless cyclic-root probe.
	// This does not define or endorse a public-role policy contract.
	role := func(id, effect string, principals []string, names ...string) *pms.RolePolicy {
		return &pms.RolePolicy{ID: id, Effect: effect, Principals: principals, Roles: names}
	}
	e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("direct-grant")}, RolePolicies: []*pms.RolePolicy{
		role("missing-subject", pms.Grant, nil, "A"),
		role("subject", pms.Grant, []string{"user:alice"}, "victim"),
		role("cycle1", pms.Grant, []string{"role:A"}, "B"),
		role("cycle2", pms.Grant, []string{"role:B"}, "A"),
		role("deny", pms.Deny, []string{"role:B"}, "victim"),
	}}}})
	requireRecoveredDecision(t, e, featureContext())
	if result, err := e.GetAllGrantedRoles(featureContext()); len(result) != 0 || errors.Code(err) != errors.EvalEngineError {
		t.Fatalf("cyclic roles: %v %v", result, err)
	}
	requireHealthyAfterRecovery(t, e)
}

func TestFeatureWatchPanicRecovery(t *testing.T) {
	e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("grant")}}}})
	apply(e, pms.StoreChangeEvent{Type: pms.POLICY_ADD, Content: "invalid event"})
	if allowed, reason, err := e.IsAllowed(featureContext()); allowed || reason != ads.ERROR_IN_EVALUATION || errors.Code(err) != errors.EvalCacheError {
		t.Fatalf("broken cache decision: %v %v %v", allowed, reason, err)
	}
	if roles, err := e.GetAllGrantedRoles(featureContext()); len(roles) != 0 || err == nil {
		t.Fatalf("broken cache roles: %v %v", roles, err)
	}
	if permissions, err := e.GetAllGrantedPermissions(featureContext()); len(permissions) != 0 || err == nil {
		t.Fatalf("broken cache permissions: %v %v", permissions, err)
	}
	apply(e, pms.StoreChangeEvent{Type: pms.FULL_RELOAD, Content: &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("healthy")}}}}})
	requireHealthyAfterRecovery(t, e)
}

type panicStartupStore struct {
	pms.PolicyStoreManagerADS
	stopped   bool
	panicStop bool
}

func (*panicStartupStore) ReadPolicyStore() (*pms.PolicyStore, error) {
	return &pms.PolicyStore{Services: []*pms.Service{nil}}, nil
}
func (*panicStartupStore) Watch() (pms.StorageChangeChannel, error) {
	return make(pms.StorageChangeChannel), nil
}
func (s *panicStartupStore) StopWatch() {
	s.stopped = true
	if s.panicStop {
		panic("stop failed")
	}
}

func TestFeatureStartupPanicRecovery(t *testing.T) {
	for _, panicStop := range []bool{false, true} {
		s := &panicStartupStore{panicStop: panicStop}
		if evaluator, err := NewWithStore(&cfg.Config{EnableWatch: true}, s); evaluator != nil || errors.Code(err) != errors.EvalEngineError || !s.stopped {
			t.Fatalf("startup evaluator=%v err=%v watch stopped=%v", evaluator, err, s.stopped)
		}
	}
}

func TestFeatureRoleConditionPanicRecovery(t *testing.T) {
	role := &pms.RolePolicy{ID: "role", Effect: pms.Grant, Principals: []string{"user:alice"}, Roles: []string{"reader"}, Condition: "Crash(x)"}
	e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("grant")}, RolePolicies: []*pms.RolePolicy{role}}}})
	expression, err := govaluate.NewEvaluableExpressionWithFunctions("Crash(x)", map[string]govaluate.ExpressionFunction{"Crash": func(...interface{}) (interface{}, error) { panic("role condition") }})
	if err != nil {
		t.Fatal(err)
	}
	e.RuntimePolicyStore.RuntimeServices["app"].RolePoliciesCache.Conditions["role"] = expression
	ctx := featureContext()
	ctx.Attributes = map[string]interface{}{"x": 1.0}
	requireRecoveredDecision(t, e, ctx)
	if roles, err := e.GetAllGrantedRoles(ctx); len(roles) != 0 || errors.Code(err) != errors.EvalEngineError {
		t.Fatalf("role panic: %v %v", roles, err)
	}
	requireHealthyAfterRecovery(t, e)
}

func TestFeatureConditionStackExhaustionPrevention(t *testing.T) {
	if os.Getenv("SPEEDLE_CONDITION_LIMIT_CHILD") != "1" {
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "-test.run=^TestFeatureConditionStackExhaustionPrevention$", "-test.timeout=40s")
		cmd.Env = append(os.Environ(), "SPEEDLE_CONDITION_LIMIT_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bounded condition process: %v\n%s", err, output)
		}
		return
	}
	previous := debug.SetMaxStack(8 << 20)
	defer debug.SetMaxStack(previous)
	for _, condition := range []string{strings.Repeat("(", 10000) + "true" + strings.Repeat(")", 10000), strings.Repeat("true && ", 20000) + "true"} {
		broken := recoveryGrant("broken")
		broken.Condition = condition
		e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("fallback"), broken}}}})
		requireRecoveredDecision(t, e, featureContext())
		requireHealthyAfterRecovery(t, e)
	}
}

type panicReadStore struct{ pms.PolicyStoreManagerADS }

func (*panicReadStore) ReadPolicyStore() (*pms.PolicyStore, error) { panic("read failed") }

func TestFeatureRefreshPanicRecovery(t *testing.T) {
	e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("grant")}}}})
	original := e.Store
	e.Store = &panicReadStore{original}
	requireEvaluationError(t, e.Refresh())
	e.Store = original
	requireHealthyAfterRecovery(t, e)
	if evaluator, err := NewFromConfig(nil); evaluator != nil || errors.Code(err) != errors.EvalEngineError {
		t.Fatalf("nil configuration: %v %v", evaluator, err)
	}
}

func TestFeatureBackgroundRecovery(t *testing.T) {
	e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("grant")}}}})
	rt := e.RuntimePolicyStore
	service := rt.RuntimeServices["app"]
	service.PoliciesCache.Conditions = nil
	service.RolePoliciesCache.Conditions = nil
	expression, err := govaluate.NewEvaluableExpression("true")
	if err != nil {
		t.Fatal(err)
	}
	// Each updater is the body of an engine-owned goroutine, so it requires its
	// own boundary rather than relying on the caller's defer.
	updatePolicyCondition(rt, "app", &pms.Policy{ID: "p"}, expression)
	updateRolePolicyCondition(rt, "app", &pms.RolePolicy{ID: "r"}, expression)
	original := rt.FunctionResultCache
	rt.FunctionResultCache = nil
	e.CleanExpiredFunctionResult()
	rt.FunctionResultCache = original
	requireHealthyAfterRecovery(t, e)
}

func TestFeatureAssertionPreservesNormalInputs(t *testing.T) {
	for _, principals := range [][]*ads.Principal{nil, {}} {
		e := featureEvaluator(t, &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("grant")}}}})
		e.SetAsserterFunc(func(ctx *ads.RequestContext) error {
			if (ctx.Subject.Principals == nil) != (principals == nil) {
				return fmt.Errorf("nil/empty principal input changed")
			}
			ctx.Subject.Principals = []*ads.Principal{{Type: "user", Name: "alice"}}
			return nil
		})
		ctx := featureContext()
		ctx.Subject.Principals = principals
		ctx.Subject.TokenType = "test"
		ctx.Subject.Token = "token"
		if allowed, _, err := e.IsAllowed(ctx); !allowed || err != nil {
			t.Fatalf("normal assertion: %v %v", allowed, err)
		}
		if !ctx.Subject.Asserted || len(ctx.Subject.Principals) != 1 || ctx.Subject.Principals[0].Name != "alice" {
			t.Fatalf("successful assertion not visible: %+v", ctx.Subject)
		}
		e.SetAsserterFunc(func(ctx *ads.RequestContext) error {
			ctx.Subject = &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "alice"}}}
			return nil
		})
		direct := featureContext()
		direct.Subject.TokenType = "test"
		direct.Subject.Token = "token"
		previous := direct.Subject
		if err := e.AssertToken(&direct); err != nil || direct.Subject == previous || !direct.Subject.Asserted {
			t.Fatalf("replacement subject semantics changed: %+v %v", direct.Subject, err)
		}
	}
}

type partialSyncFailureStore struct{ pms.PolicyStoreManagerADS }

func (s *partialSyncFailureStore) GetPolicy(service, id string) (*pms.Policy, error) {
	if id == "2" {
		policy := recoveryGrant("2")
		policy.Permissions[0].Resource = "unrelated"
		return policy, nil
	}
	return nil, errors.New(errors.StoreError, "injected store failure after one update")
}

func TestFeaturePartialSyncErrorFailsClosed(t *testing.T) {
	snapshot := &pms.PolicyStore{Services: []*pms.Service{{Name: "app", Policies: []*pms.Policy{recoveryGrant("1")}}}}
	e := featureEvaluator(t, snapshot)
	if allowed, _, err := e.IsAllowed(featureContext()); !allowed || err != nil {
		t.Fatalf("control decision: %v %v", allowed, err)
	}
	original := e.Store
	e.Store = &partialSyncFailureStore{original}
	defer func() { e.Store = original }()
	// The new store no longer contains grant 1. Insertion of 2 succeeds, then
	// fetching 3 fails before the old grant's deletion. This is a partial write.
	apply(e, pms.StoreChangeEvent{Type: pms.SYNC_RELOAD, Content: []interface{}{[]string{"app"}, map[int]string{2: "app", 3: "app"}, map[int]string{}, []string{}}})
	cache := e.RuntimePolicyStore.RuntimeServices["app"].PoliciesCache.PolicyMap
	if cache["1"] == nil || cache["2"] == nil {
		t.Fatal("test did not reach the intended partial mutation")
	}
	if allowed, reason, err := e.IsAllowed(featureContext()); allowed || reason != ads.ERROR_IN_EVALUATION || errors.Code(err) != errors.EvalCacheError {
		t.Fatalf("partial cache granted access: %v %v %v", allowed, reason, err)
	}
	if permissions, err := e.GetAllGrantedPermissions(featureContext()); len(permissions) != 0 || err == nil {
		t.Fatalf("partial permissions escaped: %v %v", permissions, err)
	}
	apply(e, pms.StoreChangeEvent{Type: pms.SERVICE_ADD, Content: snapshot.Services[0]})
	if allowed, _, err := e.IsAllowed(featureContext()); allowed || err == nil {
		t.Fatal("incremental update cleared failed-cache state")
	}
	apply(e, pms.StoreChangeEvent{Type: pms.FULL_RELOAD, Content: snapshot})
	requireHealthyAfterRecovery(t, e)
}
