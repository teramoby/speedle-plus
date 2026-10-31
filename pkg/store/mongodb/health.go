// Licensed under the Universal Permissive License (UPL) Version 1.0.
package mongodb

import (
	"context"
	stderrors "errors"

	"github.com/teramoby/speedle-plus/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (s *Store) CheckHealth(ctx context.Context) error {
	var result bson.M
	err := s.client.Database(s.Database).Collection("services").FindOne(ctx, bson.D{},
		options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}})).Decode(&result)
	if stderrors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return errors.New(errors.StoreError, "MongoDB policy store unavailable")
	}
	return nil
}
