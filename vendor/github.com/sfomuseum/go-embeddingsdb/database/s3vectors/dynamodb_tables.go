package s3vectors

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func DynamoDBTables(table_name string, metadata_table_name string) map[string]*dynamodb.CreateTableInput {

	tables := map[string]*dynamodb.CreateTableInput{
		"s3vectors_metadata": {
			TableName: aws.String(metadata_table_name),
			// Define the PK and SK attributes
			AttributeDefinitions: []types.AttributeDefinition{
				{
					AttributeName: aws.String("PK"),
					AttributeType: types.ScalarAttributeTypeS,
				},
				{
					AttributeName: aws.String("SK"),
					AttributeType: types.ScalarAttributeTypeS,
				},
			},
			// Define the primary key schema
			KeySchema: []types.KeySchemaElement{
				{
					AttributeName: aws.String("PK"),
					KeyType:       types.KeyTypeHash,
				},
				{
					AttributeName: aws.String("SK"),
					KeyType:       types.KeyTypeRange,
				},
			},
			// Define the Inverted GSI
			GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
				{
					IndexName: aws.String("GSI1"),
					KeySchema: []types.KeySchemaElement{
						{
							AttributeName: aws.String("SK"), // Inverted: SK becomes PK
							KeyType:       types.KeyTypeHash,
						},
						{
							AttributeName: aws.String("PK"), // Inverted: PK becomes SK
							KeyType:       types.KeyTypeRange,
						},
					},
					Projection: &types.Projection{
						ProjectionType: types.ProjectionTypeAll,
					},
				},
			},
			BillingMode: types.BillingModePayPerRequest,
		},
		"s3vectors": {
			TableName: aws.String(table_name),
			AttributeDefinitions: []types.AttributeDefinition{
				{
					AttributeName: aws.String("Key"),
					AttributeType: types.ScalarAttributeTypeS,
				},
				{
					AttributeName: aws.String("Provider"),
					AttributeType: types.ScalarAttributeTypeS,
				},
				{
					AttributeName: aws.String("Model"),
					AttributeType: types.ScalarAttributeTypeS,
				},
			},
			KeySchema: []types.KeySchemaElement{
				{
					AttributeName: aws.String("Key"),
					KeyType:       types.KeyTypeHash,
				},
			},
			GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
				{
					IndexName: aws.String("by_provider_model"),
					KeySchema: []types.KeySchemaElement{
						{
							AttributeName: aws.String("Provider"),
							KeyType:       types.KeyTypeHash,
						},
						{
							AttributeName: aws.String("Model"),
							KeyType:       types.KeyTypeRange,
						},
					},
					Projection: &types.Projection{
						ProjectionType: types.ProjectionTypeAll,
					},
				},
				{
					IndexName: aws.String("by_model_provider"),
					KeySchema: []types.KeySchemaElement{
						{
							AttributeName: aws.String("Model"),
							KeyType:       types.KeyTypeHash,
						},
						{
							AttributeName: aws.String("Provider"),
							KeyType:       types.KeyTypeRange,
						},
					},
					Projection: &types.Projection{
						ProjectionType: types.ProjectionTypeAll,
					},
				},
			},
			BillingMode: types.BillingModePayPerRequest,
		},
	}
	return tables
}
