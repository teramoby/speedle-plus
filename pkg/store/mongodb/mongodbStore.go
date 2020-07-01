package mongodb

import (
	"context"
	"strings"
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
	client   *mongo.Client
	Database string
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

// WritePolicyStore writes policies to a file
func (s *Store) WritePolicyStore(ps *pms.PolicyStore) error {
	err := s.DeleteServices()
	if err != nil {
		return err
	}
	err = s.DeleteFunctions()
	if err != nil {
		return err
	}
	for _, service := range ps.Services {
		err = s.CreateService(service)
		if err != nil {
			return err
		}
	}
	for _, f := range ps.Functions {
		_, err = s.CreateFunction(f)
		if err != nil {
			return err
		}
	}
	return nil

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

	return services, err

}

// GetServiceNames reads all the service names
func (s *Store) GetServiceNames() ([]string, error) {
	serviceCollection := s.client.Database(s.Database).Collection("services")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	matchstag := bson.D{{"$match", bson.D{{"$exists", true}}}}
	projectstag := bson.D{{"$project", bson.D{{"_id", 1}}}}
	opts := options.Aggregate().SetMaxTime(2 * time.Second)
	cur, err := serviceCollection.Aggregate(ctx, mongo.Pipeline{matchstag, projectstag}, opts)
	if err != nil {
		return nil, err
	}
	var services []pms.Service
	if err = cur.All(ctx, &services); err != nil {
		return nil, err
	}
	names := []string{}
	if services == nil || len(services) == 0 {
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
	if results == nil || len(results) == 0 {
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
	var result pms.Service
	result = *service
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
	err := serviceCollection.Drop(ctx)
	return err
}

func (s *Store) Watch() (pms.StorageChangeChannel, error) {
	log.Info("Enter Watch...")
	streamOptions := options.ChangeStream().SetFullDocument(options.UpdateLookup)
	changeStream, err := s.client.Database(s.Database).Watch(context.TODO(), mongo.Pipeline{}, streamOptions)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	var storeChangeChan pms.StorageChangeChannel
	storeChangeChan = make(chan pms.StoreChangeEvent)

	go func() {
		defer func() {
			changeStream.Close(context.TODO())
			close(storeChangeChan)
		}()

		for changeStream.Next(context.TODO()) {
			// A new event variable should be declared for each event.
			var event bson.M
			if err := changeStream.Decode(&event); err != nil {
				log.Error(err)
				continue
			}
			log.Info("-----watched event:", event)
			log.Info("---------- fulldocument:", event["fullDocument"])
			var ns bson.M
			ns = event["ns"].(bson.M)

			//ns.coll =="services"
			if ns["coll"] == "services" {
				//operationType == update
				if event["operationType"] == "update" {
					log.Info("===update service")
					id := time.Now().Unix()
					var service pms.Service
					docb, err := bson.Marshal(event["fullDocument"])
					if err != nil {
						log.Error(err)
						continue
					}
					err = bson.Unmarshal(docb, &service)
					if err != nil {
						log.Error(err)
						continue
					}

					serviceDeleteEvent := pms.StoreChangeEvent{Type: pms.SERVICE_DELETE, ID: id, Content: []string{service.Name}}
					log.Info("serviceDeleteEvent:", serviceDeleteEvent)
					storeChangeChan <- serviceDeleteEvent
					id = time.Now().Unix()
					serviceAddEvent := pms.StoreChangeEvent{Type: pms.SERVICE_ADD, ID: id, Content: &service}
					log.Info("serviceAddEvent:", serviceAddEvent)
					storeChangeChan <- serviceAddEvent

				} else if event["operationType"] == "insert" {
					log.Info("===insert service")
					id := time.Now().Unix()
					var service pms.Service
					docb, err := bson.Marshal(event["fullDocument"])
					if err != nil {
						log.Error(err)
						continue
					}
					err = bson.Unmarshal(docb, &service)
					if err != nil {
						log.Error(err)
						continue
					}
					serviceAddEvent := pms.StoreChangeEvent{Type: pms.SERVICE_ADD, ID: id, Content: &service}
					log.Info("###serviceAddEvent:", serviceAddEvent)
					storeChangeChan <- serviceAddEvent

				} else if event["operationType"] == "delete" {
					log.Info("===delete service")
					id := time.Now().Unix()
					serviceName := event["documentKey"].(bson.M)["_id"].(string)
					serviceDeleteEvent := pms.StoreChangeEvent{Type: pms.SERVICE_DELETE, ID: id, Content: []string{serviceName}}
					log.Info("###serviceDeleteEvent:", serviceDeleteEvent)
					storeChangeChan <- serviceDeleteEvent

				}
			} else if ns["coll"] == "functions" {

				if event["operationType"] == "insert" {
					log.Info("===insert function")
					id := time.Now().Unix()
					var f pms.Function
					docb, err := bson.Marshal(event["fullDocument"])
					if err != nil {
						log.Error(err)
						continue
					}
					err = bson.Unmarshal(docb, &f)
					if err != nil {
						log.Error(err)
						continue
					}
					funcAddEvent := pms.StoreChangeEvent{Type: pms.FUNCTION_ADD, ID: id, Content: &f}
					log.Info("###funcAddEvent:", funcAddEvent)
					storeChangeChan <- funcAddEvent

				} else if event["operationType"] == "delete" {
					log.Info("===delete function")
					id := time.Now().Unix()
					funcName := event["documentKey"].(bson.M)["_id"].(string)
					funcDeleteEvent := pms.StoreChangeEvent{Type: pms.FUNCTION_DELETE, ID: id, Content: []string{funcName}}
					log.Info("###funcDeleteEvent:", funcDeleteEvent)
					storeChangeChan <- funcDeleteEvent

				}
			}

		}
		log.Info("###exit for loop")

		if err := changeStream.Err(); err != nil {
			log.Error(err)
		}

	}()

	return storeChangeChan, nil

}

func (s *Store) StopWatch() {

}

func (s *Store) Type() string {
	return StoreType
}

func parseFilter(filterStr string) (bson.D, error) {
	if len(filterStr) == 0 {
		return bson.D{{"$eq", bson.A{1, 1}}}, nil
	}
	values := strings.Split(filterStr, " ")
	if len(values) == 2 {
		field := values[0]
		operator := values[1]
		switch operator {
		case "pr":
			return bson.D{{"$gt", bson.A{"$$p." + field, nil}}}, nil
		default:
			log.Error("invalid name filter:", filterStr)
			return nil, errors.Errorf(errors.InvalidRequest, "invalid filter %q", filterStr)
		}

	} else if len(values) == 3 {
		field := values[0]
		operator := values[1]
		target := values[2]
		switch operator {
		case "eq":
			return bson.D{{"$eq", bson.A{"$$p." + field, target}}}, nil
		case "co":
			return bson.D{{"$gte", bson.A{bson.D{{"$indexOfBytes", bson.A{"$$p." + field, target}}}, 0}}}, nil
		case "sw":
			return bson.D{{"$eq", bson.A{0, bson.D{{"$indexOfBytes", bson.A{"$$p." + field, target}}}}}}, nil
		case "gt":
			return bson.D{{"$gt", bson.A{"$$p." + field, target}}}, nil
		case "ge":
			return bson.D{{"$gte", bson.A{"$$p." + field, target}}}, nil
		case "lt":
			return bson.D{{"$lt", bson.A{"$$p." + field, target}}}, nil
		case "le":
			return bson.D{{"$lte", bson.A{"$$p." + field, target}}}, nil
		default:
			log.Error("invalid name filter:", filterStr)
			return nil, errors.Errorf(errors.InvalidRequest, "invalid filter %q", filterStr)
		}

	} else {
		log.Error("invalid filter string:", filterStr)
		return nil, errors.Errorf(errors.InvalidRequest, "invalid filter %q", filterStr)
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
	if services == nil || len(services) == 0 {
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
	if results == nil || len(results) == 0 {
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
	if services == nil || len(services) == 0 {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}
	if services[0].Policies == nil || len(services[0].Policies) == 0 {
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
	if services == nil || len(services) == 0 {
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
	if results == nil || len(results) == 0 {
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
	if services == nil || len(services) == 0 {
		return nil, errors.Errorf(errors.EntityNotFound, "service %q is not found", serviceName)
	}
	if services[0].RolePolicies == nil || len(services[0].RolePolicies) == 0 {
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
	err := serviceCollection.Drop(ctx)
	return err

}

func (s *Store) GetFunction(funcName string) (*pms.Function, error) {
	serviceCollection := s.client.Database(s.Database).Collection("functions")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	singleResult := serviceCollection.FindOne(ctx, bson.M{"_id": funcName})
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
	cur, err := serviceCollection.Find(ctx, bson.M{})
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
