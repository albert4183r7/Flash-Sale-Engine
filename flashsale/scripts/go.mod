module github.com/flashsale/scripts

go 1.23

require (
	github.com/flashsale/common v0.0.0
	github.com/joho/godotenv v1.5.1
	github.com/lib/pq v1.10.9
	golang.org/x/crypto v0.9.0
)

replace github.com/flashsale/common => ../common
