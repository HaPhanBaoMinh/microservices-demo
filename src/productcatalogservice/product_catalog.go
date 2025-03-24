package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/productcatalogservice/genproto"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/joho/godotenv"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"os"
)

type productCatalog struct {
	pb.UnimplementedProductCatalogServiceServer
	db        *dynamodb.Client
	tableName string
}

var extraLatency time.Duration

type Money struct {
	CurrencyCode string `dynamodbav:"currencyCode"`
	Units        int64  `dynamodbav:"units"`
	Nanos        int32  `dynamodbav:"nanos"`
}

type Product struct {
	ID          string   `dynamodbav:"id"`
	Name        string   `dynamodbav:"name"`
	Description string   `dynamodbav:"description"`
	Picture     string   `dynamodbav:"picture"`
	PriceUsd    Money    `dynamodbav:"priceUsd"`
	Categories  []string `dynamodbav:"categories"`
}

func newProductCatalog() (*productCatalog, error) {
	_ = godotenv.Load()

	if latencyStr := os.Getenv("EXTRA_LATENCY"); latencyStr != "" {
		if latency, err := time.ParseDuration(latencyStr); err == nil {
			extraLatency = latency
		}
	}

	tableName := os.Getenv("DYNAMODB_TABLE_NAME")
	localEndpoint := os.Getenv("DYNAMODB_LOCAL_ENDPOINT")

	if tableName == "" {
		return nil, fmt.Errorf("missing required environment variable: DYNAMODB_TABLE_NAME")
	}

	var cfg aws.Config
	var err error

	if localEndpoint != "" {
		cfg, err = config.LoadDefaultConfig(context.TODO(),
			config.WithEndpointResolver(aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
				return aws.Endpoint{URL: localEndpoint}, nil
			})),
		)
	} else {
		cfg, err = config.LoadDefaultConfig(context.TODO())
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &productCatalog{
		db:        dynamodb.NewFromConfig(cfg),
		tableName: tableName,
	}, nil
}

func (p *productCatalog) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (p *productCatalog) ListProducts(ctx context.Context, req *pb.Empty) (*pb.ListProductsResponse, error) {
	time.Sleep(extraLatency)
	products, err := p.getProductsFromDB(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.ListProductsResponse{Products: products}, nil
}

func (p *productCatalog) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.Product, error) {
	time.Sleep(extraLatency)
	return p.getProductByID(ctx, req.Id)
}

func (p *productCatalog) SearchProducts(ctx context.Context, req *pb.SearchProductsRequest) (*pb.SearchProductsResponse, error) {
	time.Sleep(extraLatency)
	products, err := p.getProductsFromDB(ctx)
	if err != nil {
		return nil, err
	}

	var results []*pb.Product
	for _, product := range products {
		if strings.Contains(strings.ToLower(product.Name), strings.ToLower(req.Query)) ||
			strings.Contains(strings.ToLower(product.Description), strings.ToLower(req.Query)) {
			results = append(results, product)
		}
	}
	return &pb.SearchProductsResponse{Results: results}, nil
}

func (p *productCatalog) getProductsFromDB(ctx context.Context) ([]*pb.Product, error) {
	out, err := p.db.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(p.tableName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan DynamoDB: %w", err)
	}

	var items []Product
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal items: %w", err)
	}

	var products []*pb.Product
	for _, item := range items {
		products = append(products, &pb.Product{
			Id:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			Picture:     item.Picture,
			PriceUsd:    &pb.Money{CurrencyCode: item.PriceUsd.CurrencyCode, Units: item.PriceUsd.Units, Nanos: item.PriceUsd.Nanos},
			Categories:  item.Categories,
		})
	}
	return products, nil
}

func (p *productCatalog) Watch(req *healthpb.HealthCheckRequest, ws healthpb.Health_WatchServer) error {
	return status.Errorf(codes.Unimplemented, "health check via Watch not implemented")
}

func (p *productCatalog) getProductByID(ctx context.Context, id string) (*pb.Product, error) {
	out, err := p.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(p.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get item: %w", err)
	}

	if out.Item == nil {
		return nil, fmt.Errorf("product not found")
	}

	var product Product
	if err := attributevalue.UnmarshalMap(out.Item, &product); err != nil {
		return nil, fmt.Errorf("failed to unmarshal product: %w", err)
	}

	return &pb.Product{
		Id:          product.ID,
		Name:        product.Name,
		Description: product.Description,
		Picture:     product.Picture,
		PriceUsd:    &pb.Money{CurrencyCode: product.PriceUsd.CurrencyCode, Units: product.PriceUsd.Units, Nanos: product.PriceUsd.Nanos},
		Categories:  product.Categories,
	}, nil
}
