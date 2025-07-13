module cabbage.town/trellis

go 1.23.0

toolchain go1.23.10

require (
	cabbage.town/shed.cabbage.town v0.0.0
	github.com/aws/aws-sdk-go v1.50.35
	github.com/joho/godotenv v1.5.1
)

replace cabbage.town/shed.cabbage.town => ../../shed.cabbage.town

require (
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	golang.org/x/net v0.38.0 // indirect
)
