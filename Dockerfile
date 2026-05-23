FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /sprintboard-api ./cmd/sprintboard-api/

FROM alpine:3.19
RUN apk add --no-cache sqlite
COPY --from=build /sprintboard-api /usr/local/bin/
EXPOSE 9400
ENTRYPOINT ["/usr/local/bin/sprintboard-api"]
