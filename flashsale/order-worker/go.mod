module github.com/flashsale/order-worker

go 1.23

require (
	github.com/flashsale/common v0.0.0
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
	github.com/lib/pq v1.10.9
	github.com/streadway/amqp v1.1.0
)

replace github.com/flashsale/common => ../common
