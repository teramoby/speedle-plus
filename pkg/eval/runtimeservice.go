//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package eval

import (
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/3rdparty/github.com/Knetic/govaluate"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
)

type RuntimePolicyStore struct {
	sync.RWMutex
	Functions           map[string]govaluate.ExpressionFunction
	customFunctionNames map[string]struct{}
	RuntimeServices     map[string]*RuntimeService
	FunctionResultCache *FuncResultCache
	FuncSvcEndpoint     string //endpoint in sphinx side to call external customer function
}

func NewRuntimePolicyStore() *RuntimePolicyStore {
	return &RuntimePolicyStore{
		customFunctionNames: make(map[string]struct{}),
		RuntimeServices:     make(map[string]*RuntimeService),
		FunctionResultCache: &FuncResultCache{
			Results: make(map[string]FuncResult),
		},
	}
}

type RuntimeService struct {
	sync.RWMutex
	Name              string
	Type              string
	PoliciesCache     *PolicyCacheData
	RolePoliciesCache *RolePolicyCacheData
	Functions         map[string]govaluate.ExpressionFunction
}

func (rtps *RuntimePolicyStore) init(ps *pms.PolicyStore, funcSvcEndpoint string) error {
	if err := validatePolicyStoreCustomFunctions(ps); err != nil {
		return err
	}
	if funcSvcEndpoint != "" {
		rtps.FuncSvcEndpoint = funcSvcEndpoint
	}
	// No need to lock, because this is a init method, evaluator should not be ready at this point
	rtps.customFunctionNames = getCustomFunctionNames(ps.Functions)
	rtps.Functions = convertFunctions(ps.Functions, rtps.FunctionResultCache, &rtps.FuncSvcEndpoint)
	for _, service := range ps.Services {
		rtps.RuntimeServices[service.Name] = convertService(service, rtps.Functions)
	}
	return nil
}

func (rtps *RuntimePolicyStore) reloadPolicyStore(ps *pms.PolicyStore) error {
	if err := validatePolicyStoreCustomFunctions(ps); err != nil {
		return err
	}
	// Clear all cached data first
	fncsResultCache := FuncResultCache{
		Results: make(map[string]FuncResult),
	}
	functions := convertFunctions(ps.Functions, &fncsResultCache, &rtps.FuncSvcEndpoint)
	customFunctionNames := getCustomFunctionNames(ps.Functions)
	services := make(map[string]*RuntimeService)

	for _, service := range ps.Services {
		services[service.Name] = convertService(service, functions)
	}

	// New cache items are ready here, replace all the caches.
	rtps.Lock()
	defer rtps.Unlock()
	rtps.Functions = functions
	rtps.customFunctionNames = customFunctionNames
	rtps.RuntimeServices = services
	rtps.FunctionResultCache = &fncsResultCache
	return nil
}

func (rtps *RuntimePolicyStore) addService(service *pms.Service) error {
	rtps.Lock()
	defer rtps.Unlock()
	if err := validateServiceCustomFunctions(service, rtps.customFunctionNames); err != nil {
		return err
	}
	rtService := convertService(service, rtps.Functions)
	rtps.RuntimeServices[service.Name] = rtService
	return nil
}

func (rtps *RuntimePolicyStore) deleteService(serviceName string) {
	rtps.Lock()
	defer rtps.Unlock()
	delete(rtps.RuntimeServices, serviceName)
}

func (rtps *RuntimePolicyStore) recompilePolicyConditionAtRuntime(serviceName string, policy *pms.Policy) (*govaluate.EvaluableExpression, error) {
	log.Debugf("Recompiling the condition of policy %q in service %q", policy.ID, serviceName)
	condition, err := compileCondition(policy.Condition, rtps.Functions)
	if err == nil {
		log.Debugf("Updating the condition of policy %q in service %q", policy.ID, serviceName)
		go updatePolicyCondition(rtps, serviceName, policy, condition)
	}
	return condition, err
}

func updatePolicyCondition(rtps *RuntimePolicyStore, serviceName string, policy *pms.Policy, condition *govaluate.EvaluableExpression) {
	var err error
	defer recoverEvaluation("PolicyConditionCache", &err)
	rtps.RLock()
	defer rtps.RUnlock()
	rtService, ok := rtps.RuntimeServices[serviceName]
	if !ok {
		// Service is not found
		log.Errorf("Unable find service %s in runtime cache.", serviceName)
		return
	}
	rtService.Lock()
	defer rtService.Unlock()
	rtService.PoliciesCache.Conditions[policy.ID] = condition
}

func (rtps *RuntimePolicyStore) recompileRolePolicyConditionAtRuntime(serviceName string, policy *pms.RolePolicy) (*govaluate.EvaluableExpression, error) {
	log.Debugf("Recompiling the condition of role policy %q in service %q", policy.ID, serviceName)
	condition, err := compileCondition(policy.Condition, rtps.Functions)
	if err == nil {
		log.Debugf("Updating the condition of role policy %q in service %q", policy.ID, serviceName)
		go updateRolePolicyCondition(rtps, serviceName, policy, condition)
	}
	return condition, err
}

func updateRolePolicyCondition(rtps *RuntimePolicyStore, serviceName string, policy *pms.RolePolicy, condition *govaluate.EvaluableExpression) {
	var err error
	defer recoverEvaluation("RoleConditionCache", &err)
	rtps.RLock()
	defer rtps.RUnlock()
	rtService, ok := rtps.RuntimeServices[serviceName]
	if !ok {
		// Service is not found
		log.Errorf("Unable find service %s in runtime cache.", serviceName)
		return
	}
	rtService.Lock()
	defer rtService.Unlock()
	rtService.RolePoliciesCache.Conditions[policy.ID] = condition
}

func (rtps *RuntimePolicyStore) addPolicy(serviceName string, policy *pms.Policy) error {
	rtps.RLock()
	defer rtps.RUnlock()

	if err := validateConditionCustomFunctions("policy", serviceName, policy.ID, policy.Condition, rtps.customFunctionNames); err != nil {
		return err
	}
	condition, _ := compileCondition(policy.Condition, rtps.Functions)
	rtService, ok := rtps.RuntimeServices[serviceName]
	if !ok {
		// Service is not found
		log.Errorf("Unable find service %s in runtime cache.", serviceName)
		return nil
	}
	rtService.Lock()
	// Golang garantees rtService.Unlock() is executed before rtps.RUnlock()
	defer rtService.Unlock()

	rtService.PoliciesCache.AddPolicyToCache(policy, condition)
	return nil
}

func (rtps *RuntimePolicyStore) deletePolicy(serviceName string, policyID string) {
	rtps.RLock()
	defer rtps.RUnlock()

	rtService, ok := rtps.RuntimeServices[serviceName]
	if !ok {
		// Service is not found
		log.Errorf("Unable find service %s in runtime cache.", serviceName)
		return
	}
	rtService.Lock()
	// Golang garantees rtService.Unlock() is executed before rtps.RUnlock()
	defer rtService.Unlock()

	rtService.PoliciesCache.DeletePolicyFromCache(policyID)
}

func (rtps *RuntimePolicyStore) addRolePolicy(serviceName string, rolePolicy *pms.RolePolicy) error {
	rtps.RLock()
	defer rtps.RUnlock()

	if err := validateConditionCustomFunctions("role policy", serviceName, rolePolicy.ID, rolePolicy.Condition, rtps.customFunctionNames); err != nil {
		return err
	}
	condition, _ := compileCondition(rolePolicy.Condition, rtps.Functions)
	rtService, ok := rtps.RuntimeServices[serviceName]
	if !ok {
		// Service is not found
		log.Errorf("Unable find service %s in runtime cache.", serviceName)
		return nil
	}
	rtService.Lock()
	// Golang garantees rtService.Unlock() is executed before rtps.RUnlock()
	defer rtService.Unlock()

	rtService.RolePoliciesCache.AddRolePolicyToCache(rolePolicy, condition)
	return nil
}

func (rtps *RuntimePolicyStore) deleteRolePolicy(serviceName string, rolePolicyID string) {
	rtps.RLock()
	defer rtps.RUnlock()

	rtService, ok := rtps.RuntimeServices[serviceName]
	if !ok {
		// Service is not found
		log.Errorf("Unable find service %s in runtime cache.", serviceName)
		return
	}
	rtService.Lock()
	// Golang garantees rtService.Unlock() is executed before rtps.RUnlock()
	defer rtService.Unlock()

	rtService.RolePoliciesCache.DeleteRolePolicyFromCache(rolePolicyID)
}

func (rtps *RuntimePolicyStore) addFunction(function *pms.Function) error {
	rtps.Lock()
	defer rtps.Unlock()

	if err := validateFunctionAdditionCustomFunctions(rtps, function); err != nil {
		return err
	}
	ef, err := rtps.FunctionResultCache.generateCustomerExpressionFunction(&rtps.FuncSvcEndpoint, function)
	if err == nil {
		rtps.Functions[function.Name] = ef
		rtps.customFunctionNames[function.Name] = struct{}{}
		log.Infof("loaded customer function %q.\n", function.Name)
	} else {
		log.Errorf("fail to load customer function %q, err is %v. \n", function.Name, err)
	}
	return nil
}

func (rtps *RuntimePolicyStore) deleteFunction(name string) {
	rtps.delFunc_rtps(name)
	rtps.delFunc_rtsvc()
}

func (rtps *RuntimePolicyStore) delFunc_rtps(name string) {
	rtps.Lock()
	defer rtps.Unlock()

	delete(rtps.Functions, name)
	delete(rtps.customFunctionNames, name)
	rtps.FunctionResultCache.DeleteFromCache(name)
}

func (rtps *RuntimePolicyStore) delFunc_rtsvc() {
	rtps.RLock()
	defer rtps.RUnlock()

	for _, svc := range rtps.RuntimeServices {
		svc.clearConditionsCache()
	}
}

func (rtps *RuntimePolicyStore) expireFunctionResultCache() {
	rtps.RLock()
	defer rtps.RUnlock()

	rtps.FunctionResultCache.CleanExpiredResult()
}

func compileCondition(condition string, functions map[string]govaluate.ExpressionFunction) (*govaluate.EvaluableExpression, error) {
	if len(condition) == 0 {
		return nil, nil
	}

	exp, err := govaluate.NewEvaluableExpressionWithFunctions(condition, functions)
	if err != nil {
		log.Errorf("Error parsing policy condition: %v", err)
		if _, limited := err.(*govaluate.ComplexityError); limited {
			return nil, errors.Wrap(err, errors.EvalEngineError, "policy condition exceeds evaluation limits")
		}
		return nil, err
	}
	return exp, nil
}

func convertService(service *pms.Service,
	functions map[string]govaluate.ExpressionFunction) *RuntimeService {
	rtService := RuntimeService{
		Name:              service.Name,
		Type:              service.Type,
		PoliciesCache:     NewPolicyCacheData(),
		RolePoliciesCache: NewRolePolicyCacheData(),
		Functions:         functions,
	}
	for _, policy := range service.Policies {
		condition, _ := compileCondition(policy.Condition, functions)
		rtService.PoliciesCache.AddPolicyToCache(policy, condition)
	}
	for _, rolePolicy := range service.RolePolicies {
		condition, _ := compileCondition(rolePolicy.Condition, functions)
		rtService.RolePoliciesCache.AddRolePolicyToCache(rolePolicy, condition)
	}

	return &rtService
}

func getCustomFunctionNames(functions []*pms.Function) map[string]struct{} {
	names := make(map[string]struct{}, len(functions))
	for _, function := range functions {
		if function != nil {
			names[function.Name] = struct{}{}
		}
	}
	return names
}

func convertFunctions(functions []*pms.Function, resultCache *FuncResultCache, funcSvcEndpoint *string) map[string]govaluate.ExpressionFunction {
	funcs := map[string]govaluate.ExpressionFunction{}

	//loading builtin functions
	for key, value := range builtinFunctions {
		funcs[key] = value
	}

	//loading customer functions
	for _, function := range functions {
		ef, err := resultCache.generateCustomerExpressionFunction(funcSvcEndpoint, function)
		if err == nil {
			funcs[function.Name] = ef
			log.Infof("loaded customer function %q.\n", function.Name)
		} else {
			log.Errorf("fail to load customer function %q, err is %v. \n", function.Name, err)
		}
	}
	return funcs
}

func (svc *RuntimeService) clearConditionsCache() {
	svc.Lock()
	defer svc.Unlock()
	svc.PoliciesCache.clearConditions()
	svc.RolePoliciesCache.clearConditions()
}

func (svc *RuntimeService) GetRelatedPolicyMap(subjectPrincipals []string, resource string,
	matchResource bool) map[string]*pms.Policy {
	return svc.PoliciesCache.GetRelatedPolicyMap(subjectPrincipals, resource, matchResource)
}

func (svc *RuntimeService) GetRelatedRolePolicyMap(subjectPrincipals []string, resource string) map[string]*pms.RolePolicy {
	return svc.RolePoliciesCache.GetRelatedRolePolicyMap(subjectPrincipals, resource)
}
