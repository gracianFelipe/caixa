# 004 — Categorias e categorização por regras

Status: concluída em 2026-09-17
Fase do plano: 2 (terceira de quatro: 002 ✔ · 003 ✔ · **004 categorias** · 005 Dockerfile)

## Objetivo

Todo lançamento que entra (HTTP ou importação) passa pelo classificador
determinístico: regras sobre `contraparte_norm`, precedência fixa, zero IA.
Lançamento sem regra fica `pendente` — é a fila que a Fase 3 vai perguntar
no Telegram.

## Decisões

* **Precedência fixa e totalmente ordenada** (o plano exige "nunca empate não
  resolvido"): `prioridade DESC` → tipo (`exata > prefixo > contem > regex`) →
  `len(padrao) DESC` → `id ASC`. Teste embaralha as regras e prova que o
  resultado independe da ordem de chegada.
* **Classificador no domínio** (`dominio/categorizacao`), puro: recebe a
  contraparte normalizada e a lista de regras, devolve categoria. `regexp` é
  stdlib; a regex é compilada no construtor da regra, então regra inválida
  não existe em memória.
* **Regras carregadas por requisição** (SELECT de dezenas de linhas, um
  usuário): KISS; cache é otimização prematura aqui.
* **Sementes conservadoras**: ~25 regras `contem`/`exata` para contrapartes
  brasileiras inequívocas (IFOOD, UBER, NETFLIX, DROGARIA...). Padrão curto e
  ambíguo entra como `exata`, não `contem` (EXTRA ⊄ EXTRATO). Limitação
  documentada: `Normalizar` remove dígitos, então "99" não é endereçável por
  regra — casos assim ficam para a regra aprendida da Fase 3.
* **Categoria no agregado imutável**: `Lancamento.ComCategoria(id, origem)`
  devolve cópia — não muta. Origem `pendente|regra|manual|importacao`
  espelhada no CHECK do banco.

## Entregas

1. Migração `003_categorias.sql`: `categorias` (ids fixos SMALLINT, 14
   sementes), `regras_categorizacao` (tipo, padrao, prioridade, origem
   semente/aprendida/manual, ativa, acertos/erros, UNIQUE (tipo, padrao)),
   FK `lancamentos.categoria_id`, sementes de regras.
2. `dominio/categoria`: `Categoria{ID, Nome}` e validação de nome.
3. `dominio/categorizacao`: `Regra` (construtor valida tipo/padrão/regex),
   `Classificar(norm, regras) (Resultado, bool)`, ordenação total.
4. `dominio/lancamento`: campos `CategoriaID`/`CategoriaOrigem` +
   `ComCategoria`; `Novo` nasce `pendente`.
5. `aplicacao`: portas `RepositorioDeRegras.Ativas` e
   `RepositorioDeCategorias.Listar`; `Registrar` e `Importar` classificam
   antes de salvar; serviço `Catalogo.Categorias`.
6. `saida/postgres`: `regras.go` (Ativas), `categorias.go` (Listar), INSERT/
   SELECT de lancamentos com as colunas novas.
7. `entrada/web`: resposta ganha `categoria_id`/`categoria_origem`;
   `GET /categorias`.
8. Testes: tabela + embaralhamento no classificador; fakes na aplicação;
   integração das novas consultas; httptest do endpoint novo.

## SEC-CHECK

PQ em todo SQL novo; regex de regra vem do repositório (semente/aprendida),
nunca direto de input externo sem passar pelo construtor — e `Classificar`
tem timeout implícito por regex compilada (RE2 do Go não retrocede, sem
ReDoS por construção). Log continua sem contraparte.

## Verificação

```powershell
go vet ./... ; go test ./...
. .\local.ps1 ; go run ./cmd/caixactl migrar        # aplica 003
go test -tags=integracao -count=1 ./...
# POST com contraparte "IFOOD *PEDIDO" volta categoria restaurante/regra
```
