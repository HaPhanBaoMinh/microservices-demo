package main

import (
	pb "github.com/GoogleCloudPlatform/microservices-demo/src/productcatalogservice/genproto"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"strconv"
)

func convertDynamoDBItemToProduct(item map[string]types.AttributeValue) *pb.Product {
	return &pb.Product{
		Id:          getStringValue(item["id"]),
		Name:        getStringValue(item["name"]),
		Description: getStringValue(item["description"]),
		Picture:     getStringValue(item["picture"]),
		PriceUsd: &pb.Money{
			CurrencyCode: getStringValue(item["currencyCode"]),
			Units:        getInt64Value(item["units"]),
			Nanos:        getInt32Value(item["nanos"]),
		},
		Categories: getStringSlice(item["categories"]),
	}
}

func getStringValue(av types.AttributeValue) string {
	if v, ok := av.(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

func getInt64Value(av types.AttributeValue) int64 {
	if v, ok := av.(*types.AttributeValueMemberN); ok {
		num, _ := strconv.ParseInt(v.Value, 10, 64)
		return num
	}
	return 0
}

func getInt32Value(av types.AttributeValue) int32 {
	if v, ok := av.(*types.AttributeValueMemberN); ok {
		num, _ := strconv.ParseInt(v.Value, 10, 32)
		return int32(num)
	}
	return 0
}

func getStringSlice(av types.AttributeValue) []string {
	if v, ok := av.(*types.AttributeValueMemberSS); ok {
		return v.Value
	}
	return []string{}
}
