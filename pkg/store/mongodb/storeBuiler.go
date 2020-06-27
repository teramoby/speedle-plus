//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package mongodb

import (
	"context"

	"github.com/prometheus/common/log"
	"github.com/spf13/pflag"
	"github.com/teramoby/speedle-plus/api/pms"
	"github.com/teramoby/speedle-plus/pkg/store"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	StoreType = "mongodb"

	//Following are keys of mongodb store properties
	MongoURIKey          = "MongoURI"
	MongoDatabaseNameKey = "MongoDatabase"

	MongoURIFlag          = "mongostore_uri"
	MongoDatabaseNameFlag = "mongostore_database"

	//default property values
	DefaultURI          = "mongodb://localhost:27017"
	DefaultDatabaseName = "speedleplus"
)

type MongoStoreBuilder struct{}

func (msb MongoStoreBuilder) NewStore(config map[string]interface{}) (pms.PolicyStoreManager, error) {
	mongoURI, ok := config[MongoURIKey].(string)
	if !ok {
		mongoURI = DefaultURI
	}

	mongoDatabase, ok := config[MongoDatabaseNameKey].(string)
	if !ok {
		mongoDatabase = DefaultDatabaseName
	}

	// Set client options
	clientOptions := options.Client().ApplyURI(mongoURI)

	// Connect to MongoDB
	client, err := mongo.Connect(context.TODO(), clientOptions)

	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	// Check the connection
	err = client.Ping(context.TODO(), nil)

	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	return &Store{client: client, Database: mongoDatabase}, nil

}

func (msb MongoStoreBuilder) GetStoreParams() map[string]string {
	return map[string]string{
		MongoURIFlag:          MongoURIKey,
		MongoDatabaseNameFlag: MongoDatabaseNameKey,
	}

}

func init() {
	pflag.String(MongoURIFlag, DefaultURI, "Store config: URI of mongoDB.")
	pflag.String(MongoDatabaseNameFlag, DefaultDatabaseName, "Store config: database to store speedle policy data.")

	store.Register(StoreType, MongoStoreBuilder{})
}
