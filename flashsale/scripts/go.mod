module github.com/flashsale/scripts

go 1.23

require (
	github.com/flashsale/common v0.0.0
	github.com/joho/godotenv v1.5.1
	golang.org/x/crypto v0.9.0
)

require github.com/lib/pq v1.10.9 // indirect

replace github.com/flashsale/common => ../common
