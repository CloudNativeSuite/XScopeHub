FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/xscopehub-etl ./etl/cmd/etl

FROM alpine:3.19
WORKDIR /app
COPY --from=build /out/xscopehub-etl /usr/local/bin/xscopehub-etl
COPY etl/api ./etl/api
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/xscopehub-etl"]
