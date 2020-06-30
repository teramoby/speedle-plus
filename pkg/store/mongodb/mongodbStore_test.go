package mongodb

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/cfg"
	"github.com/teramoby/speedle-plus/pkg/store"
)

var storeConfig *cfg.StoreConfig

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	var err error
	storeConfig, err = cfg.ReadStoreConfig("./mongoStoreConfig.json")
	if err != nil {
		log.Fatal("fail to read config file", err)
	}
	fmt.Println(storeConfig)
	return m.Run()
}

func TestWriteReadPolicyStore(t *testing.T) {
	store, err := store.NewStore(storeConfig.StoreType, storeConfig.StoreProps)
	if err != nil {
		t.Fatal("fail to new mongodb  store:", err)
	}

	if psOrigin, err := store.ReadPolicyStore(); err != nil {
		t.Fatal("fail to read mongodb  store:", err)
	} else {
		t.Log("existing number of apps:", len(psOrigin.Services))
	}

	var ps pms.PolicyStore
	for i := 0; i < 10; i++ {
		service := pms.Service{Name: fmt.Sprintf("app%d", i), Type: pms.TypeApplication}
		ps.Services = append(ps.Services, &service)
	}
	err = store.WritePolicyStore(&ps)
	if err != nil {
		t.Fatal("fail to write policy store:", err)
	}
	var psr *pms.PolicyStore
	psr, err = store.ReadPolicyStore()
	if err != nil {
		t.Fatal("fail to read policy store:", err)
	}
	if 10 != len(psr.Services) {
		t.Error("should have 10 applications in the store")
	}
	for _, app := range psr.Services {
		t.Log(app.Name, " ")
	}
}

func TestWriteReadDeleteService(t *testing.T) {
	store, err := store.NewStore(storeConfig.StoreType, storeConfig.StoreProps)
	if err != nil {
		t.Fatal("fail to new mongodb  store:", err)
	}
	//clean the service firstly
	err = store.DeleteServices()

	app := pms.Service{Name: "service1", Type: pms.TypeApplication}
	num := 1000
	i := 0
	for i < num {
		var rolePolicy pms.RolePolicy
		rolePolicy.Name = fmt.Sprintf("rp%d", i)
		rolePolicy.Effect = "grant"
		rolePolicy.Roles = []string{fmt.Sprintf("role%d", i)}
		rolePolicy.Principals = []string{"user:Alice"}
		app.RolePolicies = append(app.RolePolicies, &rolePolicy)
		i++
	}
	i = 0
	for i < num {
		var policy pms.Policy
		policy.Name = fmt.Sprintf("policy%d", i)
		policy.Effect = "grant"
		policy.Permissions = []*pms.Permission{
			{
				Resource: "/node1",
				Actions:  []string{"get", "create", "delete"},
			},
		}
		policy.Principals = [][]string{{"user:Alice"}}
		app.Policies = append(app.Policies, &policy)
		i++
	}
	err = store.CreateService(&app)
	if err != nil {
		t.Log("fail to create application:", err)
		t.FailNow()
	}
	appr, errr := store.GetService("service1")
	if errr != nil {
		t.Log("fail to get application:", errr)
		t.FailNow()
	}
	if "service1" != appr.Name {
		t.Log("app name should be service1")
		t.FailNow()
	}
	if pms.TypeApplication != appr.Type {
		t.Log("app type should be ", pms.TypeApplication)
		t.FailNow()
	}
	if num != len(appr.RolePolicies) {
		t.Logf("role policy number should be %d, but %d.", num, len(appr.RolePolicies))
		t.FailNow()
	}
	if num != len(appr.Policies) {
		t.Log("policy number should be ", num)
		t.FailNow()
	}
	//test create same name service
	sameNameApp := pms.Service{Name: "service1", Type: pms.TypeApplication}
	err = store.CreateService(&sameNameApp)
	if err != nil {
		fmt.Println("SAME NAME APP CREATION ERR", err)
	}

	err = store.DeleteService("service1")
	if err != nil {
		t.Log("fail to delete application:", err)
		t.FailNow()
	}
	appr, err = store.GetService("service1")
	t.Log("get non exist service:", err)
	if err == nil {
		t.Log("should fail as app is already deleted")
		t.FailNow()
	}
	err = store.DeleteService("nonexist-service")
	t.Log("delete non exist service:", err)
	if err == nil {
		t.Log("should fail as the service does not exist")
		t.FailNow()
	}
}

func TestMongoStore_GetPolicyByName(t *testing.T) {
	store, err := store.NewStore(storeConfig.StoreType, storeConfig.StoreProps)
	if err != nil {
		t.Fatal("fail to new mongodb  store:", err)
	}
	//clean the service firstly
	serviceName := "service1"

	err = store.DeleteService(serviceName)
	if err != nil {
		t.Log("deleteing service1, err:", err)
	}

	app := pms.Service{Name: serviceName, Type: pms.TypeApplication}
	num := 10
	i := 0
	for i < num {
		var policy pms.Policy
		policy.Name = fmt.Sprintf("policy%d", i)
		policy.Effect = "grant"
		policy.Permissions = []*pms.Permission{
			{
				Resource: "/node1",
				Actions:  []string{"get", "create", "delete"},
			},
		}
		policy.Principals = [][]string{{"user:Alice"}}
		app.Policies = append(app.Policies, &policy)
		i++
	}
	blankNamePolicy := pms.Policy{
		Effect: "grant",
		Permissions: []*pms.Permission{
			{
				Resource: "/node1",
				Actions:  []string{"get", "create", "delete"},
			},
		},
		Principals: [][]string{{"user:Alice"}},
	}
	app.Policies = append(app.Policies, &blankNamePolicy)
	duplicateNamePolicy := pms.Policy{
		Name:   "policy0",
		Effect: "grant",
		Permissions: []*pms.Permission{
			{
				Resource: "/node1",
				Actions:  []string{"get", "create", "delete"},
			},
		},
		Principals: [][]string{{"user:Alice"}},
	}
	app.Policies = append(app.Policies, &duplicateNamePolicy)

	err = store.CreateService(&app)
	if err != nil {
		t.Log("fail to create application:", err)
		t.FailNow()
	}
	service, errr := store.GetService(serviceName)
	if errr != nil {
		t.Log("fail to get application:", err)
		t.FailNow()
	}
	poilcyName := "policy0"

	policyArrListed, err := store.ListAllPolicies(service.Name, "name eq "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}

	if len(policyArrListed) != 2 { //2 policy0 policies
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllPolicies(service.Name, "name co "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != 2 { //2 policy0 policies
		t.Fatal("get poilcy by name didn't get expected policies! ", 2, len(policyArrListed))
	}

	policyArrListed, err = store.ListAllPolicies(service.Name, "name sw "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != 2 { //2 policy0 policies
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllPolicies(service.Name, "name gt "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != num-1 { //all policy name great than policy0
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllPolicies(service.Name, "name ge "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != num+1 { //all policy name great than or equals to policy0
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllPolicies(service.Name, "name lt "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != 1 { //1 blank name policy
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllPolicies(service.Name, "name le "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != 3 { //1 blank name policy and 2 duplicate policies
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllPolicies(service.Name, "name le ''")
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != 1 { //1 blank name policy
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllPolicies(service.Name, "name pr")
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != num+1 {
		t.Fatal("Get none blank name poclies failed! ", num+1, len(policyArrListed))
	}

}

func TestMongoStore_GetRolePolicyByName(t *testing.T) {
	store, err := store.NewStore(storeConfig.StoreType, storeConfig.StoreProps)
	if err != nil {
		t.Fatal("fail to new mongodb  store:", err)
	}
	//clean the service firstly
	serviceName := "service1"
	err = store.DeleteService(serviceName)
	if err != nil {
		t.Log("deleteing service1, err:", err)
	}

	app := pms.Service{Name: serviceName, Type: pms.TypeApplication}
	num := 100
	i := 0
	for i < num {
		var rolePolicy pms.RolePolicy
		rolePolicy.Name = fmt.Sprintf("rp%d", i)
		rolePolicy.Effect = "grant"
		rolePolicy.Roles = []string{fmt.Sprintf("role%d", i)}
		rolePolicy.Principals = []string{"user:Alice"}
		app.RolePolicies = append(app.RolePolicies, &rolePolicy)
		i++
	}
	blankNameRolePolicy := pms.RolePolicy{
		Effect:     "grant",
		Roles:      []string{fmt.Sprintf("role%d", i)},
		Principals: []string{"user:Alice"},
	}
	app.RolePolicies = append(app.RolePolicies, &blankNameRolePolicy)

	duplicateNameRolePolicy := pms.RolePolicy{
		Name:       "rp0",
		Effect:     "grant",
		Roles:      []string{fmt.Sprintf("role%d", i)},
		Principals: []string{"user:Alice"},
	}
	app.RolePolicies = append(app.RolePolicies, &duplicateNameRolePolicy)

	err = store.CreateService(&app)
	if err != nil {
		t.Log("fail to create application:", err)
		t.FailNow()
	}
	service, errr := store.GetService(serviceName)
	if errr != nil {
		t.Log("fail to get application:", err)
		t.FailNow()
	}
	poilcyName := "rp0"

	policyArrListed, err := store.ListAllRolePolicies(service.Name, "name eq "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}

	if len(policyArrListed) != 2 { //2 policy0 policies
		t.Fatal("get poilcy by name didn't get expected policies! ", 2, len(policyArrListed))
	}

	policyArrListed, err = store.ListAllRolePolicies(service.Name, "name co "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != 2 { //2 policy0 policies
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllRolePolicies(service.Name, "name sw "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != 2 { //2 policy0 policies
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllRolePolicies(service.Name, "name gt "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != num-1 { //all policy name great than policy0
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllRolePolicies(service.Name, "name ge "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != num+1 { //all policy name great than or equals to policy0
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllRolePolicies(service.Name, "name lt "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != 1 { //1 blank name policy
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllRolePolicies(service.Name, "name le "+poilcyName)
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != 3 { //1 blank name policy and 2 duplicate policies
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllRolePolicies(service.Name, "name le ''")
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != 1 { //1 blank name policy
		t.Fatal("get poilcy by name didn't get expected policies! ")
	}

	policyArrListed, err = store.ListAllRolePolicies(service.Name, "name pr")
	if err != nil {
		t.Fatal("Failed to list polices for service:", service.Name, err)
	}
	if len(policyArrListed) != num+1 {
		t.Fatal("Get none blank name poclies failed! ")
	}

}

func TestManagePolicies(t *testing.T) {
	store, err := store.NewStore(storeConfig.StoreType, storeConfig.StoreProps)
	if err != nil {
		t.Fatal("fail to new mongodb  store:", err)
	}
	//clean the service firstly
	store.DeleteService("service1")
	app := pms.Service{Name: "service1", Type: pms.TypeApplication}
	err = store.CreateService(&app)
	if err != nil {
		t.Fatal("fail to create application:", err)
	}
	var policy pms.Policy
	policy.Name = fmt.Sprintf("policy1")
	policy.Effect = "grant"
	policy.Permissions = []*pms.Permission{
		{
			Resource: "/node1",
			Actions:  []string{"get", "create", "delete"},
		},
	}
	policy.Principals = [][]string{{"user:Alice"}}
	policyR, err := store.CreatePolicy("service1", &policy)
	if err != nil {
		t.Fatal("fail to create policy:", err)
	}
	policyR1, err := store.GetPolicy("service1", policyR.ID)
	t.Log(policyR1)
	if err != nil {
		t.Fatal("fail to get policy:", err)
	}

	policies, err := store.ListAllPolicies("service1", "")
	if err != nil {
		t.Fatal("fail to list policies:", err)
	}
	if len(policies) != 1 {
		t.Fatal("should have 1 policy")
	}
	counts, err := store.GetPolicyAndRolePolicyCounts()
	if err != nil {
		t.Fatal("Fail to getCounts", err)
	}
	if counts["service1"].PolicyCount != 1 {
		t.Fatal("incorrect policy number")
	}
	if counts["service1"].RolePolicyCount != 0 {
		t.Fatal("incorrect role policy number")
	}

	_, err = store.GetPolicy("service1", "nonexistID")
	t.Log(err)
	if err == nil {
		t.Fatal("should fail to get policy")
	}

	err = store.DeletePolicy("service1", "nonexistID")
	t.Log(err)
	if err == nil {
		t.Fatal("should fail to delete policy")
	}

	err = store.DeletePolicy("service1", policyR.ID)
	if err != nil {
		t.Fatal("fail to delete policy:", err)
	}
}

func TestManageRolePolicies(t *testing.T) {
	store, err := store.NewStore(storeConfig.StoreType, storeConfig.StoreProps)
	if err != nil {
		t.Fatal("fail to new mongodb  store:", err)
	}

	//clean the service firstly
	store.DeleteService("service1")
	app := pms.Service{Name: "service1", Type: pms.TypeApplication}
	err = store.CreateService(&app)
	if err != nil {
		t.Fatal("fail to create application:", err)
	}
	var rolePolicy pms.RolePolicy
	rolePolicy.Name = "rp1"
	rolePolicy.Effect = "grant"
	rolePolicy.Roles = []string{"role1"}
	rolePolicy.Principals = []string{"user:Alice"}

	policyR, err := store.CreateRolePolicy("service1", &rolePolicy)
	if err != nil {
		t.Fatal("fail to create role policy:", err)
	}
	policyR1, err := store.GetRolePolicy("service1", policyR.ID)
	t.Log(policyR1)
	if err != nil {
		t.Fatal("fail to get role policy:", err)
	}

	rolePolicies, err := store.ListAllRolePolicies("service1", "")
	if err != nil {
		t.Fatal("fail to list role policies:", err)
	}
	if len(rolePolicies) != 1 {
		t.Fatal("should have 1 role policy")
	}

	counts, err := store.GetPolicyAndRolePolicyCounts()
	if err != nil {
		t.Fatal("Fail to getCounts", err)
	}
	if counts["service1"].PolicyCount != 0 {
		t.Fatal("incorrect policy number")
	}
	if counts["service1"].RolePolicyCount != 1 {
		t.Fatal("incorrect role policy number")
	}

	_, err = store.GetRolePolicy("service1", "nonexistID")
	t.Log(err)
	if err == nil {
		t.Fatal("should fail to get role policy")
	}

	err = store.DeleteRolePolicy("service1", "nonexistID")
	t.Log(err)
	if err == nil {
		t.Fatal("should fail to delete role policy")
	}

	err = store.DeleteRolePolicy("service1", policyR.ID)
	if err != nil {
		t.Fatal("fail to delete role policy:", err)
	}
}

func TestCheckItemsCount(t *testing.T) {
	store, err := store.NewStore(storeConfig.StoreType, storeConfig.StoreProps)
	if err != nil {
		t.Fatal("fail to new mongodb  store:", err)
	}

	// clean the services
	store.DeleteServices()

	// Create service1
	app1 := pms.Service{Name: "service1", Type: pms.TypeApplication}
	err = store.CreateService(&app1)
	if err != nil {
		t.Fatal("fail to create service:", err)
	}
	// Check service count
	serviceCount, err := store.GetServiceCount()
	if err != nil {
		t.Fatal("Failed to get service count:", err)
	}
	if serviceCount != 1 {
		t.Fatalf("Service count doesn't match, expected: 1, actual: %d", serviceCount)
	}

	// Create policies
	policies := []pms.Policy{
		{Name: "p01", Effect: "grant", Principals: [][]string{{"user:user1"}}},
		{Name: "p02", Effect: "grant", Principals: [][]string{{"user:user2"}}},
		{Name: "p03", Effect: "grant", Principals: [][]string{{"user:user3"}}},
	}
	for _, policy := range policies {
		_, err := store.CreatePolicy("service1", &policy)
		if err != nil {
			t.Fatal("fail to create policy:", err)
		}
	}
	// Check policy count
	policyCount, err := store.GetPolicyCount("service1")
	if err != nil {
		t.Fatal("Failed to get the policy count: ", err)
	}
	if policyCount != int64(len(policies)) {
		t.Fatalf("Policy count doesn't match, expected:%d, actual:%d", len(policies), policyCount)
	}

	// Create Role Policies
	rolePolicies := []pms.RolePolicy{
		{Name: "p01", Effect: "grant", Principals: []string{"user:user1"}, Roles: []string{"role1"}},
		{Name: "p02", Effect: "grant", Principals: []string{"user:user2"}, Roles: []string{"role2"}},
	}
	for _, rolePolicy := range rolePolicies {
		_, err := store.CreateRolePolicy("service1", &rolePolicy)
		if err != nil {
			t.Fatal("Failed to get role policy count:", err)
		}
	}
	// Check role Policy count
	rolePolicyCount, err := store.GetRolePolicyCount("service1")
	if err != nil {
		t.Fatal("Failed to get the role policy count")
	}
	if rolePolicyCount != int64(len(rolePolicies)) {
		t.Fatalf("RolePolicy count doesn't match, expected:%d, actual:%d", len(rolePolicies), rolePolicyCount)
	}

	// Create service2
	app2 := pms.Service{Name: "service2", Type: pms.TypeApplication}
	err = store.CreateService(&app2)
	if err != nil {
		t.Fatal("fail to create service:", err)
	}
	// Check service count
	serviceCount, err = store.GetServiceCount()
	if err != nil {
		t.Fatal("Failed to get service count:", err)
	}
	if serviceCount != 2 {
		t.Fatalf("Service count doesn't match, expected: 2, actual: %d", serviceCount)
	}

	// Create policies in service2
	for _, policy := range policies {
		_, err := store.CreatePolicy("service2", &policy)
		if err != nil {
			t.Fatal("fail to create policy:", err)
		}
	}
	// Check policy count in service2
	policyCount, err = store.GetPolicyCount("service2")
	if err != nil {
		t.Fatal("Failed to get the policy count: ", err)
	}
	if policyCount != int64(len(policies)) {
		t.Fatalf("Policy count doesn't match, expected:%d, actual:%d", len(policies), policyCount)
	}
	// Check policy count in both service1 and service2
	policyCount, err = store.GetPolicyCount("")
	if err != nil {
		t.Fatal("Failed to get the policy count: ", err)
	}
	if policyCount != int64(len(policies)*2) {
		t.Fatalf("Policy count doesn't match, expected:%d, actual:%d", len(policies)*2, policyCount)
	}

	// Create rolePolicy in service2
	for _, rolePolicy := range rolePolicies {
		_, err := store.CreateRolePolicy("service2", &rolePolicy)
		if err != nil {
			t.Fatal("Failed to get role policy count:", err)
		}
	}
	// Check role Policy count in service2
	rolePolicyCount, err = store.GetRolePolicyCount("service2")
	if err != nil {
		t.Fatal("Failed to get the role policy count")
	}
	if rolePolicyCount != int64(len(rolePolicies)) {
		t.Fatalf("RolePolicy count doesn't match, expected:%d, actual:%d", len(rolePolicies), rolePolicyCount)
	}
	// Check role Policy count in both service1 and service2
	rolePolicyCount, err = store.GetRolePolicyCount("")
	if err != nil {
		t.Fatal("Failed to get the role policy count")
	}
	if rolePolicyCount != int64(len(rolePolicies)*2) {
		t.Fatalf("RolePolicy count doesn't match, expected:%d, actual:%d", len(rolePolicies)*2, rolePolicyCount)
	}
	counts, err := store.GetPolicyAndRolePolicyCounts()
	if err != nil {
		t.Fatal("Fail to getCounts", err)
	}
	if (counts["service1"].PolicyCount != int64(len(policies))) ||
		(counts["service2"].PolicyCount != int64(len(policies))) {
		t.Fatal("incorrect policy number")
	}
	if (counts["service1"].RolePolicyCount != int64(len(rolePolicies))) ||
		(counts["service1"].RolePolicyCount != int64(len(rolePolicies))) {
		t.Fatal("incorrect role policy number")
	}
	fmt.Println(counts)
}
