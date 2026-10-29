package mongodb

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/suid"
)

type Store struct {
	client      *mongo.Client
	Database    string
	watchMu     sync.Mutex
	watchCancel context.CancelFunc
	watchDone   chan struct{}
}

// ReadPolicyStore reads policy store from a file
func (s *Store) ReadPolicyStore() (*pms.PolicyStore, error) {

	var ps pms.PolicyStore
	services, err := s.ListAllServices()
	if err != nil {
		return nil, err
	}
	ps.Services = services
	functions, err := s.ListAllFunctions("")
	if err != nil {
		return nil, err
	}
	ps.Functions = functions
	return &ps, nil

}

// WritePolicyStore atomically replaces the snapshot. MongoDB transactions require
// a replica set or sharded cluster; unsupported deployments fail without deleting data.
func (s *Store) WritePolicyStore(ps *pms.PolicyStore) error {
	if ps == nil {
		return errors.New(errors.InvalidRequest, "policy store must not be nil")
	}
	// Encode before starting the transaction, so invalid input never removes data.
	services := make([]interface{}, 0, len(ps.Services))
	for _, service := range ps.Services {
		if service == nil {
			return errors.New(errors.InvalidRequest, "service must not be nil")
		}
		data, err := bson.Marshal(service)
		if err != nil {
			return err
		}
		var copy pms.Service
		if err := bson.Unmarshal(data, &copy); err != nil {
			return err
		}
		for _, policy := range copy.Policies {
			if policy == nil {
				return errors.New(errors.InvalidRequest, "policy must not be nil")
			}
			if policy.ID == "" {
				policy.ID = suid.New().String()
			}
		}
		for _, policy := range copy.RolePolicies {
			if policy == nil {
				return errors.New(errors.InvalidRequest, "role policy must not be nil")
			}
			if policy.ID == "" {
				policy.ID = suid.New().String()
			}
		}
		services = append(services, &copy)
	}
	functions := make([]interface{}, 0, len(ps.Functions))
	for _, function := range ps.Functions {
		if function == nil {
			return errors.New(errors.InvalidRequest, "function must not be nil")
		}
		if err := validateFunc(function); err != nil {
			return err
		}
		data, err := bson.Marshal(function)
		if err != nil {
			return err
		}
		functions = append(functions, bson.Raw(data))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	session, err := s.client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())
	_, err = session.WithTransaction(ctx, func(ctx mongo.SessionContext) (interface{}, error) {
		database := s.client.Database(s.Database)
		serviceCollection := database.Collection("services")
		functionCollection := database.Collection("functions")
		if _, err := serviceCollection.DeleteMany(ctx, bson.D{}); err != nil {
			return nil, err
		}
		if _, err := functionCollection.DeleteMany(ctx, bson.D{}); err != nil {
			return nil, err
		}
		if len(services) > 0 {
			if _, err := serviceCollection.InsertMany(ctx, services); err != nil {
				return nil, err
			}
		}
		if len(functions) > 0 {
			if _, err := functionCollection.InsertMany(ctx, functions); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	return err
}

// ListAllServices lists all the services
func (s *Store) ListAllServices() ([]*pms.Service, error) {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cur, err := serviceCollection.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	services := []*pms.Service{}
	for cur.Next(ctx) {
		var service pms.Service
		err := cur.Decode(&service)
		if err != nil {
			return nil, err
		}
		services = append(services, &service)
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}
	return services, nil

}

// GetServiceNames reads all the service names
func (s *Store) GetServiceNames() ([]string, error) {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	projectstag := bson.D{{"$project", bson.D{{"_id", 1}}}}
	opts := options.Aggregate().SetMaxTime(2 * time.Second)
	cur, err := serviceCollection.Aggregate(ctx, mongo.Pipeline{projectstag}, opts)
	if err != nil {
		return nil, err
	}
	var services []pms.Service
	if err = cur.All(ctx, &services); err != nil {
		return nil, err
	}
	names := []string{}
	if len(services) == 0 {
		return names, nil
	}
	for _, service := range services {
		names = append(names, service.Name)
	}

	return names, nil

}

// GetPolicyAndRolePolicyCounts returns a map, in which the key is the service name, and the value is the count of both policies and role policies in the service.
func (s *Store) GetPolicyAndRolePolicyCounts() (map[string]*pms.PolicyAndRolePolicyCount, error) {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	matchstag := bson.D{{"$match", bson.D{}}}
	projectstag := bson.D{{"$project",
		bson.D{{"policyCount", bson.D{{"$size", bson.D{{"$ifNull", bson.A{"$policies", bson.A{}}}}}}},
			{"rolepolicyCount", bson.D{{"$size", bson.D{{"$ifNull", bson.A{"$rolepolicies", bson.A{}}}}}}},
			{"count", bson.D{{"$sum",
				bson.A{
					bson.D{{"$size", bson.D{{"$ifNull", bson.A{"$policies", bson.A{}}}}}},
					bson.D{{"$size", bson.D{{"$ifNull", bson.A{"$rolepolicies", bson.A{}}}}}},
				}}}},
		}}}

	opts := options.Aggregate().SetMaxTime(5 * time.Second)
	cur, err := serviceCollection.Aggregate(ctx, mongo.Pipeline{matchstag, projectstag}, opts)
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err = cur.All(ctx, &results); err != nil {
		return nil, err
	}
	countMap := make(map[string]*pms.PolicyAndRolePolicyCount)
	if len(results) == 0 {
		return countMap, nil
	}
	for _, res := range results {
		var counts pms.PolicyAndRolePolicyCount
		counts.PolicyCount = int64(res["policyCount"].(int32))
		counts.RolePolicyCount = int64(res["rolepolicyCount"].(int32))
		countMap[res["_id"].(string)] = &counts
	}

	return countMap, nil

}

// GetServiceCount gets the service count
func (s *Store) GetServiceCount() (int64, error) {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	num, err := serviceCollection.CountDocuments(ctx, bson.D{})
	if err != nil {
		return -1, err
	}

	return num, nil
}

// GetService gets the detailed info of a service
func (s *Store) GetService(serviceName string) (*pms.Service, error) {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	singleResult := serviceCollection.FindOne(ctx, bson.M{"_id": serviceName})
	if singleResult.Err() != nil {
		if singleResult.Err() == mongo.ErrNoDocuments {
			return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
		} else {
			return nil, singleResult.Err()
		}

	}
	var service *pms.Service
	err := singleResult.Decode(&service)

	return service, err

}

func generateID(service *pms.Service) (*pms.Service, error) {
	var result pms.Service = *service
	if result.Policies == nil {
		result.Policies = []*pms.Policy{}
	}
	if result.RolePolicies == nil {
		result.RolePolicies = []*pms.RolePolicy{}
	}
	for _, policy := range result.Policies {
		policy.ID = suid.New().String()
	}
	for _, rolePolicy := range result.RolePolicies {
		rolePolicy.ID = suid.New().String()
	}
	return &result, nil
}

// CreateService creates a new service
func (s *Store) CreateService(service *pms.Service) error {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serviceWithID, _ := generateID(service)
	insertResult, err := serviceCollection.InsertOne(ctx, serviceWithID)
	if mongo.IsDuplicateKeyError(err) {
		return errors.Errorf(errors.EntityAlreadyExists, "service %q already exists", service.Name)
	}
	if err != nil {
		return err
	}
	log.Info(insertResult.InsertedID)
	return nil
}

// DeleteService deletes a service named ${serviceName} from a file
func (s *Store) DeleteService(serviceName string) error {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	deleteResult, err := serviceCollection.DeleteOne(ctx, bson.M{"_id": serviceName})
	if err != nil {
		return err
	}
	if deleteResult.DeletedCount == 0 {
		return errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}
	return nil
}

// DeleteServices deletes all services from a file
func (s *Store) DeleteServices() error {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := serviceCollection.DeleteMany(ctx, bson.D{})
	return err
}

// Watch streams committed changes until StopWatch is called or the stream fails.
func (s *Store) Watch() (pms.StorageChangeChannel, error) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.watchCancel != nil {
		return nil, errors.New(errors.StoreError, "policy store is already being watched")
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := s.client.Database(s.Database).Watch(ctx, mongo.Pipeline{}, options.ChangeStream().SetFullDocument(options.UpdateLookup))
	if err != nil {
		cancel()
		return nil, err
	}
	changes := make(pms.StorageChangeChannel)
	done := make(chan struct{})
	s.watchCancel, s.watchDone = cancel, done
	go func() {
		defer close(done)
		defer close(changes)
		defer cancel()
		defer func() {
			closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer closeCancel()
			if err := stream.Close(closeCtx); err != nil {
				log.Error(err)
			}
		}()
		for stream.Next(ctx) {
			var event mongoChangeEvent
			if err := stream.Decode(&event); err != nil {
				log.Error(err)
				continue
			}
			events, err := event.storeEvents()
			if err != nil {
				log.Error(err)
				continue
			}
			for _, change := range events {
				select {
				case changes <- change:
				case <-ctx.Done():
					return
				}
			}
		}
		if err := stream.Err(); err != nil && ctx.Err() == nil {
			log.Error(err)
		}
	}()
	return changes, nil
}

// StopWatch also unblocks a watcher waiting for a consumer to receive an event.
func (s *Store) StopWatch() {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	if s.watchCancel != nil {
		s.watchCancel()
		<-s.watchDone
		s.watchCancel, s.watchDone = nil, nil
	}
}

type mongoChangeEvent struct {
	Operation string `bson:"operationType"`
	Namespace struct {
		Collection string `bson:"coll"`
	} `bson:"ns"`
	DocumentKey struct {
		ID string `bson:"_id"`
	} `bson:"documentKey"`
	FullDocument bson.Raw `bson:"fullDocument"`
}

func (e mongoChangeEvent) storeEvents() ([]pms.StoreChangeEvent, error) {
	change := func(kind pms.EventType, content interface{}) pms.StoreChangeEvent {
		return pms.StoreChangeEvent{Type: kind, ID: time.Now().UnixNano(), Content: content}
	}
	if e.Operation == "dropDatabase" || e.Operation == "invalidate" {
		return []pms.StoreChangeEvent{change(pms.FULL_RELOAD, nil)}, nil
	}
	collection := e.Namespace.Collection
	if collection != "services" && collection != "functions" {
		return nil, nil
	}
	if e.Operation == "drop" || e.Operation == "rename" {
		return []pms.StoreChangeEvent{change(pms.FULL_RELOAD, nil)}, nil
	}
	if e.Operation == "delete" {
		kind := pms.SERVICE_DELETE
		if collection == "functions" {
			kind = pms.FUNCTION_DELETE
		}
		return []pms.StoreChangeEvent{change(kind, []string{e.DocumentKey.ID})}, nil
	}
	if e.Operation != "insert" && e.Operation != "update" && e.Operation != "replace" {
		return nil, nil
	}
	// UpdateLookup may return null if a later operation already deleted the document.
	// Reload rather than publishing an empty service or silently retaining stale data.
	if len(e.FullDocument) == 0 {
		return []pms.StoreChangeEvent{change(pms.FULL_RELOAD, nil)}, nil
	}
	if collection == "functions" {
		var function pms.Function
		if err := bson.Unmarshal(e.FullDocument, &function); err != nil {
			return nil, err
		}
		return []pms.StoreChangeEvent{change(pms.FUNCTION_ADD, &function)}, nil
	}
	var service pms.Service
	if err := bson.Unmarshal(e.FullDocument, &service); err != nil {
		return nil, err
	}
	events := []pms.StoreChangeEvent{}
	if e.Operation != "insert" {
		events = append(events, change(pms.SERVICE_DELETE, []string{service.Name}))
	}
	return append(events, change(pms.SERVICE_ADD, &service)), nil
}

// Close stops watching and releases the MongoDB client's connections.
func (s *Store) Close() error {
	s.StopWatch()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.client.Disconnect(ctx)
}

func (s *Store) Type() string {
	return StoreType
}

// parseFilter follows the shared name-filter contract. Unknown fields/operators
// are ignored, and target strings are literals rather than aggregation paths.
func parseFilter(filterStr string) (bson.D, error) {
	all := bson.D{{"$eq", bson.A{1, 1}}}
	values := strings.Split(filterStr, " ")
	if len(values) < 2 || values[0] != "name" {
		return all, nil
	}
	value := bson.D{{"$ifNull", bson.A{"$$p.name", ""}}}
	target := ""
	if len(values) > 2 {
		target = values[2]
	}
	literal := bson.D{{"$literal", target}}
	switch values[1] {
	case "pr":
		return bson.D{{"$ne", bson.A{value, ""}}}, nil
	case "eq":
		return bson.D{{"$eq", bson.A{value, literal}}}, nil
	case "co":
		return bson.D{{"$gte", bson.A{bson.D{{"$indexOfBytes", bson.A{value, literal}}}, 0}}}, nil
	case "sw":
		return bson.D{{"$eq", bson.A{0, bson.D{{"$indexOfBytes", bson.A{value, literal}}}}}}, nil
	case "gt":
		return bson.D{{"$gt", bson.A{value, literal}}}, nil
	case "ge":
		return bson.D{{"$gte", bson.A{value, literal}}}, nil
	case "lt":
		return bson.D{{"$lt", bson.A{value, literal}}}, nil
	case "le":
		return bson.D{{"$lte", bson.A{value, literal}}}, nil
	default:
		return all, nil
	}
}

// For policy manager
func (s *Store) ListAllPolicies(serviceName string, filter string) ([]*pms.Policy, error) {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	matchstag := bson.D{{"$match", bson.D{{"_id", serviceName}}}}
	condition, err := parseFilter(filter)
	if err != nil {
		return nil, err
	}
	projectstag := bson.D{
		{"$project", bson.D{
			{"policies", bson.D{
				{"$filter", bson.D{
					{"input", "$policies"},
					{"as", "p"},
					{"cond", condition}},
				}},
			}},
		}}
	opts := options.Aggregate().SetMaxTime(2 * time.Second)
	cur, err := serviceCollection.Aggregate(ctx, mongo.Pipeline{matchstag, projectstag}, opts)
	if err != nil {
		return nil, err
	}
	var services []pms.Service
	if err = cur.All(ctx, &services); err != nil {
		return nil, errors.New(errors.StoreError, err.Error())
	}
	if len(services) == 0 {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}

	return services[0].Policies, nil

}

func (s *Store) GetPolicyCount(serviceName string) (int64, error) {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var matchstag bson.D
	if len(serviceName) != 0 {
		matchstag = bson.D{{"$match", bson.D{{"_id", serviceName}}}}
	} else {
		matchstag = bson.D{{"$match", bson.D{}}}
	}

	projectstag := bson.D{{"$project", bson.D{{"policycount", bson.D{{"$size", bson.D{{"$ifNull", bson.A{"$policies", bson.A{}}}}}}}}}}

	opts := options.Aggregate().SetMaxTime(5 * time.Second)
	cur, err := serviceCollection.Aggregate(ctx, mongo.Pipeline{matchstag, projectstag}, opts)
	if err != nil {
		return -1, err
	}
	var results []bson.M
	if err = cur.All(ctx, &results); err != nil {
		return -1, err
	}
	if len(results) == 0 {
		if len(serviceName) != 0 {
			return -1, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
		} else {
			return 0, nil
		}
	}
	policyCount := int64(0)

	for _, res := range results {
		policyCount += int64(res["policycount"].(int32))
	}

	return policyCount, nil

}

func (s *Store) GetPolicy(serviceName string, id string) (*pms.Policy, error) {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	matchstag := bson.D{{"$match", bson.D{{"_id", serviceName}}}}
	projectstag := bson.D{
		{"$project", bson.D{
			{"policies", bson.D{
				{"$filter", bson.D{
					{"input", "$policies"},
					{"as", "p"},
					{"cond", bson.D{{"$eq", bson.A{"$$p._id", id}}}}},
				}},
			}},
		}}
	opts := options.Aggregate().SetMaxTime(5 * time.Second)
	cur, err := serviceCollection.Aggregate(ctx, mongo.Pipeline{matchstag, projectstag}, opts)
	if err != nil {
		return nil, err
	}
	var services []pms.Service
	if err = cur.All(ctx, &services); err != nil {
		return nil, err
	}
	if len(services) == 0 {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}
	if len(services[0].Policies) == 0 {
		return nil, errors.Errorf(errors.EntityNotFound, "policy %q is not found", id)
	}
	return services[0].Policies[0], nil

}

func (s *Store) DeletePolicy(serviceName string, id string) error {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.D{{"_id", serviceName}}
	update := bson.D{{"$pull", bson.D{{"policies", bson.D{{"_id", id}}}}}}
	result, err := serviceCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}
	if result.ModifiedCount == 0 {
		return errors.Errorf(errors.EntityNotFound, "policy %q is not found", id)
	}
	return nil

}

func (s *Store) DeletePolicies(serviceName string) error {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.D{{"_id", serviceName}}
	update := bson.D{{"$pull", bson.D{{"policies", bson.D{{"$exists", true}}}}}}
	result := serviceCollection.FindOneAndUpdate(ctx, filter, update)
	if result.Err() == mongo.ErrNoDocuments {
		return errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	} else {
		return result.Err()
	}

}

func (s *Store) CreatePolicy(serviceName string, policy *pms.Policy) (*pms.Policy, error) {
	dupPolicy := *policy
	dupPolicy.ID = suid.New().String()
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.D{{"_id", serviceName}}
	update := bson.D{{"$push", bson.D{{"policies", dupPolicy}}}}
	result := serviceCollection.FindOneAndUpdate(ctx, filter, update)
	if result.Err() == nil {
		return &dupPolicy, nil
	} else if result.Err() == mongo.ErrNoDocuments {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	} else {
		return nil, result.Err()
	}

}

// For role policy manager
func (s *Store) ListAllRolePolicies(serviceName string, filter string) ([]*pms.RolePolicy, error) {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	matchstag := bson.D{{"$match", bson.D{{"_id", serviceName}}}}
	condition, err := parseFilter(filter)
	if err != nil {
		return nil, err
	}
	projectstag := bson.D{
		{"$project", bson.D{
			{"rolepolicies", bson.D{
				{"$filter", bson.D{
					{"input", "$rolepolicies"},
					{"as", "p"},
					{"cond", condition}},
				}},
			}},
		}}
	opts := options.Aggregate().SetMaxTime(2 * time.Second)
	cur, err := serviceCollection.Aggregate(ctx, mongo.Pipeline{matchstag, projectstag}, opts)
	if err != nil {
		return nil, err
	}
	var services []pms.Service
	if err = cur.All(ctx, &services); err != nil {
		return nil, errors.New(errors.StoreError, err.Error())
	}
	if len(services) == 0 {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}

	return services[0].RolePolicies, nil

}

func (s *Store) GetRolePolicyCount(serviceName string) (int64, error) {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var matchstag bson.D
	if len(serviceName) != 0 {
		matchstag = bson.D{{"$match", bson.D{{"_id", serviceName}}}}
	} else {
		matchstag = bson.D{{"$match", bson.D{}}}
	}

	projectstag := bson.D{{"$project", bson.D{{"policycount", bson.D{{"$size", bson.D{{"$ifNull", bson.A{"$rolepolicies", bson.A{}}}}}}}}}}

	opts := options.Aggregate().SetMaxTime(5 * time.Second)
	cur, err := serviceCollection.Aggregate(ctx, mongo.Pipeline{matchstag, projectstag}, opts)
	if err != nil {
		return -1, err
	}
	var results []bson.M
	if err = cur.All(ctx, &results); err != nil {
		return -1, err
	}
	if len(results) == 0 {
		if len(serviceName) != 0 {
			return -1, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
		} else {
			return 0, nil
		}
	}
	policyCount := int64(0)

	for _, res := range results {
		policyCount += int64(res["policycount"].(int32))
	}

	return policyCount, nil

}

func (s *Store) GetRolePolicy(serviceName string, id string) (*pms.RolePolicy, error) {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	matchstag := bson.D{{"$match", bson.D{{"_id", serviceName}}}}
	projectstag := bson.D{
		{"$project", bson.D{
			{"rolepolicies", bson.D{
				{"$filter", bson.D{
					{"input", "$rolepolicies"},
					{"as", "p"},
					{"cond", bson.D{{"$eq", bson.A{"$$p._id", id}}}}},
				}},
			}},
		}}
	opts := options.Aggregate().SetMaxTime(5 * time.Second)
	cur, err := serviceCollection.Aggregate(ctx, mongo.Pipeline{matchstag, projectstag}, opts)
	if err != nil {
		return nil, err
	}
	var services []pms.Service
	if err = cur.All(ctx, &services); err != nil {
		return nil, err
	}
	if len(services) == 0 {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}
	if len(services[0].RolePolicies) == 0 {
		return nil, errors.Errorf(errors.EntityNotFound, "rolepolicy %q is not found", id)
	}
	return services[0].RolePolicies[0], nil

}

func (s *Store) DeleteRolePolicy(serviceName string, id string) error {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.D{{"_id", serviceName}}
	update := bson.D{{"$pull", bson.D{{"rolepolicies", bson.D{{"_id", id}}}}}}
	result, err := serviceCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}
	if result.ModifiedCount == 0 {
		return errors.Errorf(errors.EntityNotFound, "rolepolicy %q is not found", id)
	}
	return nil

}

func (s *Store) DeleteRolePolicies(serviceName string) error {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.D{{"_id", serviceName}}
	update := bson.D{{"$pull", bson.D{{"rolepolicies", bson.D{{"$exists", true}}}}}}
	result := serviceCollection.FindOneAndUpdate(ctx, filter, update)
	if result.Err() == mongo.ErrNoDocuments {
		return errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	} else {
		return result.Err()
	}

}

func (s *Store) CreateRolePolicy(serviceName string, rolePolicy *pms.RolePolicy) (*pms.RolePolicy, error) {
	dupPolicy := *rolePolicy
	dupPolicy.ID = suid.New().String()
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.D{{"_id", serviceName}}
	update := bson.D{{"$push", bson.D{{"rolepolicies", dupPolicy}}}}
	result := serviceCollection.FindOneAndUpdate(ctx, filter, update)
	if result.Err() == nil {
		return &dupPolicy, nil
	} else if result.Err() == mongo.ErrNoDocuments {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	} else {
		return nil, result.Err()
	}
}

func validateFunc(function *pms.Function) error {
	if function.Name == "" || function.FuncURL == "" {
		return errors.New(errors.InvalidRequest, "\"name\" and \"funcURL\" in function definition can not be empty")
	}
	return nil
}

func (s *Store) CreateFunction(function *pms.Function) (*pms.Function, error) {
	if err := validateFunc(function); err != nil {
		return nil, err
	}
	serviceCollection := s.client.Database(s.Database).Collection("functions")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	insertResult, err := serviceCollection.InsertOne(ctx, function)
	if mongo.IsDuplicateKeyError(err) {
		return nil, errors.Errorf(errors.EntityAlreadyExists, "function %q already exists", function.Name)
	}
	if err != nil {
		return nil, err
	}
	log.Info(insertResult.InsertedID)
	return function, nil

}

func (s *Store) DeleteFunction(funcName string) error {
	serviceCollection := s.client.Database(s.Database).Collection("functions")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	deleteResult, err := serviceCollection.DeleteOne(ctx, bson.M{"_id": funcName})
	if err != nil {
		return err
	}
	if deleteResult.DeletedCount == 0 {
		return errors.Errorf(errors.EntityNotFound, "function %q is not found", funcName)
	}
	return nil

}

func (s *Store) DeleteFunctions() error {
	serviceCollection := s.client.Database(s.Database).Collection("functions")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := serviceCollection.DeleteMany(ctx, bson.D{})
	return err

}

func (s *Store) GetFunction(funcName string) (*pms.Function, error) {
	serviceCollection := s.client.Database(s.Database).Collection("functions")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	singleResult := serviceCollection.FindOne(ctx, bson.M{"_id": funcName})
	if singleResult.Err() == mongo.ErrNoDocuments {
		return nil, errors.Errorf(errors.EntityNotFound, "function %q is not found", funcName)
	}
	if singleResult.Err() != nil {
		return nil, singleResult.Err()
	}
	var f *pms.Function
	err := singleResult.Decode(&f)

	return f, err

}

func (s *Store) ListAllFunctions(filter string) ([]*pms.Function, error) {
	serviceCollection := s.client.Database(s.Database).Collection("functions")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	condition, err := parseFilter(filter)
	if err != nil {
		return nil, err
	}
	query := bson.D{{"$expr", bson.D{{"$let", bson.D{
		{"vars", bson.D{{"p", bson.D{{"name", "$_id"}}}}},
		{"in", condition},
	}}}}}
	cur, err := serviceCollection.Find(ctx, query)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	functions := []*pms.Function{}
	for cur.Next(ctx) {
		var f pms.Function
		err := cur.Decode(&f)
		if err != nil {
			return nil, err
		}
		functions = append(functions, &f)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return functions, nil

}

func (s *Store) GetFunctionCount() (int64, error) {
	serviceCollection := s.client.Database(s.Database).Collection("functions")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	num, err := serviceCollection.CountDocuments(ctx, bson.D{})
	if err != nil {
		return -1, err
	}

	return num, nil

}
