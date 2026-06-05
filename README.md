# Blindando o Job de Auditoria com Testes

Projeto em Go que processa arquivos de logs JSON e gera um relatório consolidado de auditoria. Esta fase dá continuidade ao desafio "Acelerando o Job de Auditoria", adicionando uma suíte de testes automatizados para proteger a lógica de agregação, parsing e concorrência.

## Objetivo

O foco desta etapa é garantir que mudanças futuras não reintroduzam race conditions nem quebrem a corretude dos relatórios.

A suíte cobre:

- testes unitários do `Report`;
- teste de estresse concorrente com `sync.WaitGroup`;
- table-driven tests para o processamento de arquivos;
- comparação entre o pipeline com worker pool e a implementação sequencial;
- validação com o race detector do Go.

## Estrutura

```text
.
├── exemplo.go      # GenerateMockFiles(), gerador de logs mock
├── go.mod          # módulo Go
├── main.go         # implementação do processamento e execução comparativa
├── main_test.go    # suíte de testes automatizados
├── README.md       # visão geral do projeto
└── TESTING.md      # comandos de execução, testes e race detector
```

## Como funciona

Cada linha válida dos arquivos de log é um JSON com o tipo do evento e a região:

```json
{"event_type":"click","region":"us-east-1"}
```

Linhas inválidas são contabilizadas como erro de parsing.

O relatório (`Report`) consolida:

- total de eventos válidos;
- total de erros;
- contagem por tipo de evento: `click`, `view`, `purchase`, `login`;
- contagem por região: `us-east-1`, `eu-west-1`, `ap-southeast-2`, `sa-east-1`.

## Implementações

O projeto mantém quatro estratégias para comparação:

- `ProcessSequential`: baseline sequencial.
- `ProcessConcurrentNaive`: versão concorrente propositalmente insegura, usada para demonstrar race condition.
- `ProcessConcurrentMutex`: versão concorrente protegida por `sync.Mutex`.
- `ProcessPipeline`: worker pool com channels e agregador único.

As versões sequencial, mutex e pipeline devem gerar relatórios idênticos. A versão ingênua pode produzir dados corrompidos e deve acusar race quando executada com `go run -race .`.

## Testes implementados

O arquivo `main_test.go` contém:

- `TestReportAddition`: valida `AddEvent` e `AddError`.
- `TestReportConcurrency`: dispara 1000 goroutines usando `AddEventSafe` e `AddErrorSafe`.
- `TestProcessFileHelper`: usa table-driven tests e `t.Run()` para validar linhas válidas, inválidas, dimensões desconhecidas e arquivo ausente.
- `TestProcessPipelineMatchesSequential`: confirma que o pipeline gera o mesmo relatório da versão sequencial.

## Execução rápida

```bash
go test ./...
```

Com detector de race:

```bash
go test -race ./...
```

Para executar o processamento comparativo:

```bash
go run .
```

Consulte [TESTING.md](./TESTING.md) para comandos completos, exemplos com datasets menores e validação da race condition proposital.
