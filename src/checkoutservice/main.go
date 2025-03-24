package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"cloud.google.com/go/profiler"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/checkoutservice/genproto"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"database/sql"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	usdCurrency = "USD"
)

var log *logrus.Logger

var (
	db *sql.DB

	// Table names from .env
	tableOrders         string
	tableOrderItems     string
	tableOrderPayments  string
	tableOrderShipments string
)

func init() {
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Error loading .env file: %v", err) // Now log is initialized
	}

	// Get database connection details
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	// Get table names from .env
	tableOrders = os.Getenv("TABLE_ORDERS")
	tableOrderItems = os.Getenv("TABLE_ORDER_ITEMS")
	tableOrderPayments = os.Getenv("TABLE_ORDER_PAYMENTS")
	tableOrderShipments = os.Getenv("TABLE_ORDER_SHIPMENTS")

	// Create connection string
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	// Connect to database
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Test the connection
	if err = db.Ping(); err != nil {
		log.Fatalf("Database connection error: %v", err)
	}

	// Create tables if they do not exist
	createTables()
}

func createTables() {
	createOrdersTable := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, tableOrders)

	createOrderItemsTable := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id SERIAL PRIMARY KEY,
			order_id UUID REFERENCES %s(id) ON DELETE CASCADE,
			product_id TEXT NOT NULL,
			product_name TEXT NOT NULL,
			unit_price NUMERIC(10,2) NOT NULL,
			quantity INT NOT NULL CHECK (quantity > 0),
			subtotal NUMERIC(10,2) GENERATED ALWAYS AS (unit_price * quantity) STORED
		)`, tableOrderItems, tableOrders)

	createOrderPaymentsTable := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			order_id UUID PRIMARY KEY REFERENCES %s(id) ON DELETE CASCADE,
			transaction_id UUID NOT NULL,
			status VARCHAR(20) DEFAULT 'PENDING',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, tableOrderPayments, tableOrders)

	createOrderShipmentsTable := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			order_id UUID PRIMARY KEY REFERENCES %s(id) ON DELETE CASCADE,
			tracking_id TEXT NOT NULL,
			status VARCHAR(20) DEFAULT 'PROCESSING',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`, tableOrderShipments, tableOrders)

	// Execute table creation queries
	queries := []string{createOrdersTable, createOrderItemsTable, createOrderPaymentsTable, createOrderShipmentsTable}
	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			log.Fatalf("Error creating table: %v", err)
		}
	}
}

type checkoutService struct {
	pb.UnimplementedCheckoutServiceServer

	productCatalogSvcAddr string
	productCatalogSvcConn *grpc.ClientConn

	cartSvcAddr string
	cartSvcConn *grpc.ClientConn

	currencySvcAddr string
	currencySvcConn *grpc.ClientConn

	shippingSvcAddr string
	shippingSvcConn *grpc.ClientConn

	emailSvcAddr string
	emailSvcConn *grpc.ClientConn

	paymentSvcAddr string
	paymentSvcConn *grpc.ClientConn
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Warnf("failed to load .env file: %v", err)
	}

	ctx := context.Background()
	if os.Getenv("ENABLE_TRACING") == "1" {
		log.Info("Tracing enabled.")
		initTracing()

	} else {
		log.Info("Tracing disabled.")
	}

	if os.Getenv("ENABLE_PROFILER") == "1" {
		log.Info("Profiling enabled.")
		go initProfiling("checkoutservice", "1.0.0")
	} else {
		log.Info("Profiling disabled.")
	}

	port := os.Getenv("PORT")
	if os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}

	svc := new(checkoutService)
	mustMapEnv(&svc.shippingSvcAddr, "SHIPPING_SERVICE_ADDR")
	mustMapEnv(&svc.productCatalogSvcAddr, "PRODUCT_CATALOG_SERVICE_ADDR")
	mustMapEnv(&svc.cartSvcAddr, "CART_SERVICE_ADDR")
	mustMapEnv(&svc.currencySvcAddr, "CURRENCY_SERVICE_ADDR")
	mustMapEnv(&svc.emailSvcAddr, "EMAIL_SERVICE_ADDR")
	mustMapEnv(&svc.paymentSvcAddr, "PAYMENT_SERVICE_ADDR")

	mustConnGRPC(ctx, &svc.shippingSvcConn, svc.shippingSvcAddr)
	mustConnGRPC(ctx, &svc.productCatalogSvcConn, svc.productCatalogSvcAddr)
	mustConnGRPC(ctx, &svc.cartSvcConn, svc.cartSvcAddr)
	mustConnGRPC(ctx, &svc.currencySvcConn, svc.currencySvcAddr)
	mustConnGRPC(ctx, &svc.emailSvcConn, svc.emailSvcAddr)
	mustConnGRPC(ctx, &svc.paymentSvcConn, svc.paymentSvcAddr)

	log.Infof("service config: %+v", svc)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatal(err)
	}

	var srv *grpc.Server

	// Propagate trace context always
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{}))
	srv = grpc.NewServer(
		grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
		grpc.StreamInterceptor(otelgrpc.StreamServerInterceptor()),
	)

	pb.RegisterCheckoutServiceServer(srv, svc)
	healthpb.RegisterHealthServer(srv, svc)
	log.Infof("starting to listen on tcp: %q", lis.Addr().String())
	err = srv.Serve(lis)
	log.Fatal(err)
}

func initStats() {
	//TODO(arbrown) Implement OpenTelemetry stats
}

func initTracing() {
	var (
		collectorAddr string
		collectorConn *grpc.ClientConn
	)

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*3)
	defer cancel()

	mustMapEnv(&collectorAddr, "COLLECTOR_SERVICE_ADDR")
	mustConnGRPC(ctx, &collectorConn, collectorAddr)

	exporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithGRPCConn(collectorConn))
	if err != nil {
		log.Warnf("warn: Failed to create trace exporter: %v", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)

}

func initProfiling(service, version string) {
	// TODO(ahmetb) this method is duplicated in other microservices using Go
	// since they are not sharing packages.
	for i := 1; i <= 3; i++ {
		if err := profiler.Start(profiler.Config{
			Service:        service,
			ServiceVersion: version,
			// ProjectID must be set if not running on GCP.
			// ProjectID: "my-project",
		}); err != nil {
			log.Warnf("failed to start profiler: %+v", err)
		} else {
			log.Info("started Stackdriver profiler")
			return
		}
		d := time.Second * 10 * time.Duration(i)
		log.Infof("sleeping %v to retry initializing Stackdriver profiler", d)
		time.Sleep(d)
	}
	log.Warn("could not initialize Stackdriver profiler after retrying, giving up")
}

func mustMapEnv(target *string, envKey string) {
	v := os.Getenv(envKey)
	if v == "" {
		panic(fmt.Sprintf("environment variable %q not set", envKey))
	}
	*target = v
}

func mustConnGRPC(ctx context.Context, conn **grpc.ClientConn, addr string) {
	var err error
	ctx, cancel := context.WithTimeout(ctx, time.Second*3)
	defer cancel()
	*conn, err = grpc.DialContext(ctx, addr,
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor()))
	if err != nil {
		panic(errors.Wrapf(err, "grpc: failed to connect %s", addr))
	}
}

func (cs *checkoutService) Check(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

func (cs *checkoutService) Watch(req *healthpb.HealthCheckRequest, ws healthpb.Health_WatchServer) error {
	return status.Errorf(codes.Unimplemented, "health check via Watch not implemented")
}

func (cs *checkoutService) PlaceOrder(ctx context.Context, req *pb.PlaceOrderRequest) (*pb.PlaceOrderResponse, error) {
	log.Infof("[PlaceOrder] user_id=%q user_currency=%q", req.UserId, req.UserCurrency)

	// Generate a new order UUID
	orderID, err := uuid.NewUUID()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate order uuid")
	}

	// Save order with status "PENDING"
	_, err = db.Exec(fmt.Sprintf(`INSERT INTO %s (order_id, status, created_at, updated_at) VALUES ($1, 'PENDING', $2, $3)`, tableOrders),
		orderID.String(), time.Now(), time.Now(),
	)
	if err != nil {
		log.Errorf("failed to insert order: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to save order")
	}

	// Save order payment with status "PENDING"
	_, err = db.Exec(fmt.Sprintf(`INSERT INTO %s (order_id, status) VALUES ($1, 'PENDING')`, tableOrderPayments),
		orderID.String(),
	)
	if err != nil {
		log.Errorf("failed to insert order payment: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to save order payment")
	}

	// Process payment
	txID, err := cs.chargeCard(ctx, &pb.Money{CurrencyCode: req.UserCurrency}, req.CreditCard)
	if err != nil {
		// Update order payment to "FAILED"
		_, _ = db.Exec(fmt.Sprintf(`UPDATE %s SET status = 'FAILED' WHERE order_id = $1`, tableOrderPayments),
			orderID.String(),
		)

		// Update order status to "FAILED"
		_, _ = db.Exec(fmt.Sprintf(`UPDATE %s SET status = 'FAILED', updated_at = $1 WHERE order_id = $2`, tableOrders),
			time.Now(), orderID.String(),
		)

		return nil, status.Errorf(codes.Internal, "failed to charge card: %+v", err)
	}
	log.Infof("payment successful (transaction_id: %s)", txID)

	// Update payment status to "SUCCESS"
	_, err = db.Exec(fmt.Sprintf(`UPDATE %s SET transaction_id = $1, status = 'SUCCESS' WHERE order_id = $2`, tableOrderPayments),
		txID, orderID.String(),
	)
	if err != nil {
		log.Errorf("failed to update payment status to SUCCESS: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to update payment record")
	}

	// Update order status to "PROCESSING"
	_, err = db.Exec(fmt.Sprintf(`UPDATE %s SET status = 'PROCESSING', updated_at = $1 WHERE order_id = $2`, tableOrders),
		time.Now(), orderID.String(),
	)
	if err != nil {
		log.Errorf("failed to update order status to PROCESSING: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to update order")
	}

	// Ship order
	shippingTrackingID, err := cs.shipOrder(ctx, req.Address, nil)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "shipping error: %+v", err)
	}

	// Save shipping details
	_, err = db.Exec(fmt.Sprintf(`INSERT INTO %s (order_id, tracking_id, status) VALUES ($1, $2, 'SHIPPED')`, tableOrderShipments),
		orderID.String(), shippingTrackingID,
	)
	if err != nil {
		log.Errorf("failed to insert order shipment: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to save shipment info")
	}

	// Return response
	resp := &pb.PlaceOrderResponse{
		Order: &pb.OrderResult{
			OrderId:            orderID.String(),
			ShippingTrackingId: shippingTrackingID,
		},
	}
	return resp, nil
}

type orderPrep struct {
	orderItems            []*pb.OrderItem
	cartItems             []*pb.CartItem
	shippingCostLocalized *pb.Money
}

func (cs *checkoutService) prepareOrderItemsAndShippingQuoteFromCart(ctx context.Context, userID, userCurrency string, address *pb.Address) (orderPrep, error) {
	var out orderPrep
	cartItems, err := cs.getUserCart(ctx, userID)
	if err != nil {
		return out, fmt.Errorf("cart failure: %+v", err)
	}
	orderItems, err := cs.prepOrderItems(ctx, cartItems, userCurrency)
	if err != nil {
		return out, fmt.Errorf("failed to prepare order: %+v", err)
	}
	shippingUSD, err := cs.quoteShipping(ctx, address, cartItems)
	if err != nil {
		return out, fmt.Errorf("shipping quote failure: %+v", err)
	}
	shippingPrice, err := cs.convertCurrency(ctx, shippingUSD, userCurrency)
	if err != nil {
		return out, fmt.Errorf("failed to convert shipping cost to currency: %+v", err)
	}

	out.shippingCostLocalized = shippingPrice
	out.cartItems = cartItems
	out.orderItems = orderItems
	return out, nil
}

func (cs *checkoutService) quoteShipping(ctx context.Context, address *pb.Address, items []*pb.CartItem) (*pb.Money, error) {
	shippingQuote, err := pb.NewShippingServiceClient(cs.shippingSvcConn).
		GetQuote(ctx, &pb.GetQuoteRequest{
			Address: address,
			Items:   items})
	if err != nil {
		return nil, fmt.Errorf("failed to get shipping quote: %+v", err)
	}
	return shippingQuote.GetCostUsd(), nil
}

func (cs *checkoutService) getUserCart(ctx context.Context, userID string) ([]*pb.CartItem, error) {
	cart, err := pb.NewCartServiceClient(cs.cartSvcConn).GetCart(ctx, &pb.GetCartRequest{UserId: userID})
	if err != nil {
		return nil, fmt.Errorf("failed to get user cart during checkout: %+v", err)
	}
	return cart.GetItems(), nil
}

func (cs *checkoutService) emptyUserCart(ctx context.Context, userID string) error {
	if _, err := pb.NewCartServiceClient(cs.cartSvcConn).EmptyCart(ctx, &pb.EmptyCartRequest{UserId: userID}); err != nil {
		return fmt.Errorf("failed to empty user cart during checkout: %+v", err)
	}
	return nil
}

func (cs *checkoutService) prepOrderItems(ctx context.Context, items []*pb.CartItem, userCurrency string) ([]*pb.OrderItem, error) {
	out := make([]*pb.OrderItem, len(items))
	cl := pb.NewProductCatalogServiceClient(cs.productCatalogSvcConn)

	for i, item := range items {
		product, err := cl.GetProduct(ctx, &pb.GetProductRequest{Id: item.GetProductId()})
		if err != nil {
			return nil, fmt.Errorf("failed to get product #%q", item.GetProductId())
		}
		price, err := cs.convertCurrency(ctx, product.GetPriceUsd(), userCurrency)
		if err != nil {
			return nil, fmt.Errorf("failed to convert price of %q to %s", item.GetProductId(), userCurrency)
		}
		out[i] = &pb.OrderItem{
			Item: item,
			Cost: price}
	}
	return out, nil
}

func (cs *checkoutService) convertCurrency(ctx context.Context, from *pb.Money, toCurrency string) (*pb.Money, error) {
	result, err := pb.NewCurrencyServiceClient(cs.currencySvcConn).Convert(context.TODO(), &pb.CurrencyConversionRequest{
		From:   from,
		ToCode: toCurrency})
	if err != nil {
		return nil, fmt.Errorf("failed to convert currency: %+v", err)
	}
	return result, err
}

func (cs *checkoutService) chargeCard(ctx context.Context, amount *pb.Money, paymentInfo *pb.CreditCardInfo) (string, error) {
	paymentResp, err := pb.NewPaymentServiceClient(cs.paymentSvcConn).Charge(ctx, &pb.ChargeRequest{
		Amount:     amount,
		CreditCard: paymentInfo})
	if err != nil {
		return "", fmt.Errorf("could not charge the card: %+v", err)
	}
	return paymentResp.GetTransactionId(), nil
}

func (cs *checkoutService) sendOrderConfirmation(ctx context.Context, email string, order *pb.OrderResult) error {
	_, err := pb.NewEmailServiceClient(cs.emailSvcConn).SendOrderConfirmation(ctx, &pb.SendOrderConfirmationRequest{
		Email: email,
		Order: order})
	return err
}

func (cs *checkoutService) shipOrder(ctx context.Context, address *pb.Address, items []*pb.CartItem) (string, error) {
	resp, err := pb.NewShippingServiceClient(cs.shippingSvcConn).ShipOrder(ctx, &pb.ShipOrderRequest{
		Address: address,
		Items:   items})
	if err != nil {
		return "", fmt.Errorf("shipment failed: %+v", err)
	}
	return resp.GetTrackingId(), nil
}
