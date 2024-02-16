FROM golang:1.20.4 as builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 8080

ENV HOST_ADDR=":8080"
ENV FILE_PATH="/var/data"
ENV SECRET="testing"

VOLUME ["/var/data"]

CMD ["./main"]
