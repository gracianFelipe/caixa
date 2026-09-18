# internal/dominio

## Purpose

Regras de negócio puras do Caixa: dinheiro, competência, identidade e o
agregado `Lancamento`. Nenhum pacote aqui sabe que existe HTTP, banco ou
relógio de sistema.

## Ownership

Cada subpasta é um pacote de domínio independente. Pacotes de domínio podem
importar uns aos outros (`lancamento` importa `dinheiro`, `competencia`,
`identidade`); nada de fora importa para dentro senão `aplicacao` e adaptadores.

## Local Contracts

* **Só stdlib.** Nenhum `import` fora da biblioteca padrão. Verificado na CI
  (ver raiz). Se uma solução idiomática exige lib externa (ex.:
  `golang.org/x/text` para acentos), escreve-se a versão mínima à mão.
* **Tempo e fuso entram como parâmetro.** Sem `time.Now()`, sem
  `time.LoadLocation`. Funções recebem `time.Time` e `*time.Location`.
* **Aleatoriedade entra como parâmetro** (`io.Reader`), pelo mesmo motivo.
* **Construtor valida; struct não existe inválido.** `Novo(...) (T, error)`
  checa todas as invariantes. Campos que não podem ser alterados por fora são
  privados com acessores (`Competencia`).
* **Erros são sentinela** (`var ErrX = errors.New("pacote: descricao")`),
  comparados com `errors.Is`. Mensagem em minúsculas, prefixada pelo pacote,
  sem PII.
* **Tipos próprios sobre tipos base** (`Centavos int64`, `Meio string`,
  `ID [16]byte`) em vez de structs quando o valor é um só.
* Nomes em português sem acento; identificadores técnicos em inglês.

## Work Guidance

* Todo tipo com forma textual implementa `String()` e `Analisar(string)`, e o
  teste prova a ida e volta (`TestIdaEVolta`).
* Parser de entrada externa ganha `Fuzz*` que verifica ausência de panic e
  ida e volta, não valor específico.
* Testes usam `time.FixedZone("America/Sao_Paulo", -3*60*60)`, nunca
  `LoadLocation`, para rodar em Windows sem tzdata.
* Comparar structs com `cmp.Diff`; `Competencia` exige
  `cmp.AllowUnexported(competencia.Competencia{})`.
* Tabela de casos com campo `nome` e `t.Run(c.nome, ...)`.

## Verification

```bash
go test ./internal/dominio/...
go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./internal/dominio/... | grep -v '^github.com/gracianFelipe/caixa/'
# saida vazia = conforme
```

## Child DOX Index

Nenhum. Cada pacote é pequeno o bastante para este documento cobrir.
