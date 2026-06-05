# Como testar

Este guia descreve como executar a suíte automatizada, validar o race detector e rodar o processador de logs manualmente.

## Pré-requisitos

- Go instalado.
- Terminal aberto na raiz do projeto.

Para conferir:

```bash
go version
```

## Executar a suíte de testes

```bash
go test ./...
```

Resultado esperado:

```text
ok  	acelerando-job-auditoria	...
```

A suíte cobre:

- agregação direta em `Report`;
- métodos thread-safe `AddEventSafe` e `AddErrorSafe`;
- parsing e processamento de arquivos com table-driven tests;
- equivalência entre `ProcessSequential` e `ProcessPipeline`.

## Executar com race detector

```bash
go test -race ./...
```

Resultado esperado:

```text
ok  	acelerando-job-auditoria	...
```

Esse comando valida que os testes que usam as versões seguras não produzem `WARNING: DATA RACE`.

## Executar o job manualmente

```bash
go run .
```

A execução padrão gera 100 arquivos com 10.000 linhas cada no diretório `logs/`.

Resultado esperado:

- Parte 1, sequencial: relatório correto.
- Parte 2, concorrente ingênua: pode retornar números incorretos.
- Parte 3, concorrente com mutex: mesmo relatório da parte 1.
- Parte 4, pipeline com worker pool: mesmo relatório da parte 1.

Para o volume padrão, os totais corretos esperados são:

```text
eventos=980100
erros=19900
tipos=[245025 245025 245025 245025]
regioes=[240100 250000 240000 250000]
```

As linhas finais devem confirmar as versões seguras:

```text
Parte 1 == Parte 3: true
Parte 1 == Parte 4: true
```

A comparação com a parte 2 pode variar entre `true` e `false`, porque ela é propositalmente insegura e depende do interleaving das goroutines.

## Executar com dataset menor

Para testes rápidos:

```bash
AUDIT_NUM_FILES=10 AUDIT_EVENTS_PER_FILE=1000 go run .
```

Totais corretos esperados para esse volume:

```text
eventos=9810
erros=190
tipos=[2453 2453 2452 2452]
regioes=[2410 2500 2400 2500]
```

## Variáveis de ambiente

O `main.go` aceita as seguintes variáveis:

| Variável | Padrão | Descrição |
| --- | ---: | --- |
| `AUDIT_LOG_DIR` | `logs` | Diretório onde os arquivos mock serão gerados. |
| `AUDIT_NUM_FILES` | `100` | Quantidade de arquivos de log. |
| `AUDIT_EVENTS_PER_FILE` | `10000` | Quantidade de linhas por arquivo. |
| `AUDIT_WORKERS` | `runtime.NumCPU()` | Quantidade de workers na parte 4. |
| `AUDIT_SKIP_NAIVE` | `false` | Quando `true`, pula a parte 2. Útil para rodar `-race` apenas nas versões seguras. |

Exemplo:

```bash
AUDIT_LOG_DIR=logs_demo AUDIT_NUM_FILES=20 AUDIT_EVENTS_PER_FILE=5000 AUDIT_WORKERS=4 go run .
```

## Validar as partes seguras com race detector

A parte 2 é propositalmente insegura. Para validar apenas as partes 1, 3 e 4 com o race detector, pule a versão ingênua:

```bash
AUDIT_SKIP_NAIVE=true AUDIT_LOG_DIR=logs_race_safe AUDIT_NUM_FILES=10 AUDIT_EVENTS_PER_FILE=1000 go run -race .
```

Resultado esperado:

- o comando termina com exit code `0`;
- não aparece nenhum `WARNING: DATA RACE`;
- as comparações finais mostram:

```text
Parte 1 == Parte 3: true
Parte 1 == Parte 4: true
Parte 2 ignorada por AUDIT_SKIP_NAIVE=true
```

## Demonstrar a race condition da parte 2

Use um dataset menor para o race detector terminar mais rápido:

```bash
AUDIT_LOG_DIR=logs_race_naive AUDIT_NUM_FILES=10 AUDIT_EVENTS_PER_FILE=1000 go run -race .
```

Resultado esperado:

- o Go imprime um ou mais blocos `WARNING: DATA RACE`;
- a stack trace aponta para `Report.AddEvent` ou `Report.AddError`;
- a chamada vem de `ProcessConcurrentNaive`;
- o comando termina com erro, geralmente com `exit status 66`.

Exemplo de trecho esperado:

```text
WARNING: DATA RACE
Read at ...
  main.(*Report).AddEvent()
  main.ProcessConcurrentNaive.func1.1()
```

Essa race acontece porque várias goroutines executam `report.AddEvent(event)` e `report.AddError()` ao mesmo tempo, alterando campos como `TotalEvents`, `TotalErrors`, `ByEventType` e `ByRegion` sem mutex.

## Problema de permissão no cache do Go

Se aparecer erro de permissão no cache, defina um cache local:

```bash
mkdir -p .gocache
GOCACHE="$PWD/.gocache" go test ./...
```

Com `-race`:

```bash
mkdir -p .gocache
GOCACHE="$PWD/.gocache" go test -race ./...
```

O diretório `.gocache/` já está no `.gitignore`.
