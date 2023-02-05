# syntax=docker/dockerfile:1
FROM golang:1.19.4-alpine3.17 AS build

WORKDIR /build

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o r6index_auth ./cmd

#-
FROM alpine

WORKDIR /app
COPY --from=build /build/r6index_auth .
# COPY --from=build /build/.env .

CMD [ "./r6index_auth" ]