# productcatalogservice

### Discription

- This is a simple product catalog service that provides a RESTful API to read product data from a DynamoDB table.

### To run with local dynamoDB

docker run -p 8000:8000 -d amazon/dynamodb-local -jar DynamoDBLocal.jar -inMemory -sharedDb

```

```

aws dynamodb create-table \
 --table-name Products \
 --attribute-definitions AttributeName=id,AttributeType=S \
 --key-schema AttributeName=id,KeyType=HASH \
 --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 \
 --endpoint-url http://localhost:8000 \
 --region us-east-1

```

```

aws dynamodb list-tables --endpoint-url http://localhost:8000 --region us-west-2

```

```

aws dynamodb put-item --table-name Products --item '{
"id": {"S": "OLJCESPC7Z"},
"name": {"S": "Sunglasses"},
"description": {"S": "Add a modern touch to your outfits with these sleek aviator sunglasses."},
"picture": {"S": "/static/img/products/sunglasses.jpg"},
"priceUsd": {"M": {
"currencyCode": {"S": "USD"},
"units": {"N": "19"},
"nanos": {"N": "990000000"}
}},
"categories": {"L": [{"S": "accessories"}]}
}' --endpoint-url http://localhost:8000 --region us-east-1

```

```

aws dynamodb scan --table-name Products --endpoint-url http://localhost:8000

```

```

