# Gera a config do relay (um flag set por app) a partir de flags/apps.
FROM golang:1.23-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# As flags ficam no mesmo caminho que o relay usa: o gerador monta os
# retrievers a partir de --apps-dir.
RUN mkdir -p /goff && cp -r flags /goff/flags \
 && go run . relay-config --apps-dir /goff/flags/apps -o /goff/goff-proxy.yaml

FROM gofeatureflag/go-feature-flag:v1.46.0

COPY --from=builder /goff /goff

ENTRYPOINT ["/go-feature-flag", "--config", "/goff/goff-proxy.yaml"]
