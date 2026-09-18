# Estagio de compilacao: toolchain completa, cache de modulos separado do
# codigo para a camada de dependencias so invalidar quando go.mod mudar.
FROM golang:1.27 AS compilacao

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO desligado = binario estatico; sem libc no runtime. -trimpath tira o
# caminho da maquina de build dos stacks. tzdata ja vai embutido via import.
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api \
 && go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && go build -trimpath -ldflags="-s -w" -o /out/caixactl ./cmd/caixactl

# Runtime: distroless static — sem shell, sem apt, usuario nao-root, CA certs
# inclusos (o cliente Telegram da Fase 3 fala TLS). Superficie de ataque quase
# nula: a imagem so tem os dois binarios.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=compilacao /out/api /api
COPY --from=compilacao /out/worker /worker
COPY --from=compilacao /out/caixactl /caixactl

EXPOSE 8080
ENTRYPOINT ["/api"]
