# URL Shortener Lambda

### Deploy to Lambda
1. Build `GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap main.go`
2. Zip the build `zip myFunction.zip bootstrap`
3. Upload to Lambda (Amazon Linux 2023, arm64, bootstrap, edge function)