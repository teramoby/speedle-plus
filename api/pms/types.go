//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package pms

type Permission struct {
	Resource           string   `json:"resource,omitempty"`
	ResourceExpression string   `json:"resourceExpression,omitempty"`
	Actions            []string `json:"actions,omitempty"`
}

type Function struct {
	Name           string            `json:"name" bson:"_id"`
	Description    string            `json:"description,omitempty" bson:"description,omitempty"`
	FuncURL        string            `json:"funcURL" bson:"funcurl"`                                   //used by speedle/sphinx ADS
	LocalFuncURL   string            `json:"localFuncURL,omitempty"  bson:"localfuncurl"`              //used by sphinx runtime proxy to get better performance
	CA             string            `json:"ca,omitempty" bson:"ca,omitempty"`                         //security related configurations
	ResultCachable bool              `json:"resultCachable,omitempty" bson:"resultcachable,omitempty"` //false by default
	ResultTTL      int64             `json:"resultTTL,omitempty" bson:"resultttl,omitempty"`           // TTL of function result in second
	Metadata       map[string]string `json:"metadata,omitempty" bson:"metadata,omitempty"`
}

type Policy struct {
	ID          string            `json:"id" bson:"_id"`
	Name        string            `json:"name" bson:"name,omitempty"`
	Effect      string            `json:"effect,omitempty" bson:"effect,omitempty"`
	Permissions []*Permission     `json:"permissions,omitempty" bson:"permissions,omitempty"`
	Principals  [][]string        `json:"principals,omitempty" bson:"principals,omitempty"`
	Condition   string            `json:"condition,omitempty" bson:"condition,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty" bson:"metadata,omitempty"`
}

const (
	Grant = "grant"
	Deny  = "deny"
)

const (
	TypeK8SCluster  = "k8s-cluster"
	TypeApplication = "application"
)

type RolePolicy struct {
	ID                  string            `json:"id" bson:"_id"`
	Name                string            `json:"name" bson:"name,omitempty"`
	Effect              string            `json:"effect,omitempty" bson:"effect,omitempty"`
	Roles               []string          `json:"roles,omitempty" bson:"roles,omitempty"`
	Principals          []string          `json:"principals,omitempty" bson:"principals,omitempty"`
	Resources           []string          `json:"resources,omitempty" bson:"resources,omitempty"`
	ResourceExpressions []string          `json:"resourceExpressions,omitempty" bson:"resourceexpressions,omitempty"`
	Condition           string            `json:"condition,omitempty" bson:"condition,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty" bson:"metadata,omitempty"`
}

type Service struct {
	Name         string            `json:"name" binding:"required"  bson:"_id"`
	Type         string            `json:"type,omitempty" bson:"type,omitempty"`
	Policies     []*Policy         `json:"policies,omitempty" bson:"policies,omitempty"`
	RolePolicies []*RolePolicy     `json:"rolePolicies,omitempty" bson:"rolepolicies,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty" bson:"metadata,omitempty"`
}

const GlobalService = "global"

type PolicyStore struct {
	Functions []*Function `json:"functions,omitempty"`
	Services  []*Service  `json:"services,omitempty"`
}

type PolicyAndRolePolicyCount struct {
	PolicyCount     int64 `json:"policycount,omitempty"`
	RolePolicyCount int64 `json:"rolePolicycount,omitempty"`
}

type EventType uint8

const (
	INVALID EventType = iota
	SERVICE_DELETE
	SERVICE_ADD
	POLICY_DELETE
	POLICY_ADD
	ROLEPOLICY_DELETE
	ROLEPOLICY_ADD
	FUNCTION_DELETE
	FUNCTION_ADD
	SYNC_RELOAD
	FULL_RELOAD
)

type StoreChangeEvent struct {
	Type EventType
	// Event ID
	ID int64
	// Event content.
	// In case of a delete event, the content is the identity of the deleted item, such as the application name;
	// in case of put events, the content is the value of the newly created item, like an application
	Content interface{}
}

type StoreUpdateData struct {
	ServiceName string
	Data        interface{}
}

// StorageChangeChannel is the channel through which the policy evaluator
// gets StoreChangeEvent for refreshing cache
//TODO It's better to change to pointer type @tony
type StorageChangeChannel chan StoreChangeEvent
