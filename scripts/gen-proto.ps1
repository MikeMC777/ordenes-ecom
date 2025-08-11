protoc --proto_path=proto --go_out=. --go-grpc_out=. `
  --go_opt=module=github.com/MikeMC777/ordenes-ecom `
  --go-grpc_opt=module=github.com/MikeMC777/ordenes-ecom `
  proto\user.proto