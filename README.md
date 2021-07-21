# Accept a Card Payment

Build a simple checkout form to collect card details. Included are some basic build and run scripts you can use to start up the application.

## Running the sample

1. Run the server

```
go run server.go
```

2. Go to [http://localhost:4242/checkout.html](http://localhost:4242/checkout.html)

curl -d '{"currency":"usd","PaymentMethodType":"card","userID":"11111"}' 