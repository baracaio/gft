# Plano: Dry-Run SQS Consumer GO

Pipeline de ingestão (Clientes e Bloqueios) com SQS → Lambda (Go) → DynamoDB, infra em Terraform, demo local com Floci.

## 1. Requisitos do enunciado

### Arquitetura

- 2 filas SQS (Bloqueios, CadastroClientes), cada uma com DLQ
- 2 Lambdas (uma por fila) via Event Source Mapping
- 1 tabela DynamoDB "Cadastro Geral" (PK = GUID do cliente, SK nula, 1 linha por cliente)

### Infraestrutura

- Tudo em Terraform (SQS, DLQs, Lambdas, DynamoDB)
- Uma região, uma conta AWS
- DynamoDB on-demand
- Retenção das filas: 1 dia
- Até 2 retries por mensagem; esgotados → DLQ

### Lambda

- Go
- Batch + partial batch response
- Event Source Mapping
- Mesmo zip para os dois domínios, mas 2 Lambdas físicas distintas

### Regras funcionais

- Só grava se o registro for mais recente que o persistido
- Cada domínio atualiza só suas colunas
- Pode salvar bloqueios antes de o cliente existir
- Paralelismo entre domínios resolvido com optimistic locking
- Bloqueios (campos planos no JSON) viram um campo List `{codigo, valor}`; sem códigos duplicados na mesma mensagem
- Até 100 bloqueios por cliente
- Mensagens vão para a DLQ quando os retries se esgotam

### Entrega

- Prazo de 1 semana, um único zip (código de aplicação + IaC)
- Demo com registro das horas investidas
- IA permitida, mas é preciso explicar cada linha e cada escolha

## 2. Decisões tomadas

| # | Decisão |
|---|---|
| 1 | Campo `data_atualizacao` na mensagem, tratado **por domínio**, ISO 8601 UTC (formato fixo) |
| 2 | 1 mensagem = 1 cliente |
| 3 | Lista de bloqueios substituída por inteiro, ordenada por código; acima de 100: rejeita a mensagem |
| 4 | Optimistic locking: timestamp por domínio + `version` |
| 5 | Retry interno em conflito de versão (3 tentativas, backoff curto com jitter) antes de falhar; mensagem mais antiga: descarta com sucesso |
| 6 | Batch agrupado por cliente: grupos em paralelo (pool limitado), itens do mesmo cliente em sequência (ordem por timestamp) |
| 7 | Erro permanente: reporta falha. Falha definitiva (última tentativa): também salva em tabela de falhas |
| 8 | `maxReceiveCount = 3` (1 tentativa + 2 retries) |
| 9 | Visibility timeout ≥ 6× o timeout da Lambda |
| 10 | Seleção de domínio por variável de ambiente `DOMAIN` |
| 11 | Terraform em arquivos simples por recurso |
| 12 | Arquitetura da Lambda parametrizável, padrão amd64 |
| 13 | Emulador: Floci; plano B: LocalStack (token Hobby) |
| 14 | Demo: script + vídeo/GIF curto |
| 15 | Nomes dos atributos: `data_atualizacao_cliente` e `data_atualizacao_bloqueios` |
| 16 | `version` única na linha (assumido; confirmar) |

## 3. Modelo de dados

### Tabela `cadastro_geral`

- PK `codigo_cliente` (S, GUID), sem SK, on-demand

| Atributo | Tipo | Origem |
|---|---|---|
| `numero_agencia` | N | cliente |
| `numero_conta` | N | cliente |
| `numero_dac` | N | cliente |
| `data_atualizacao_cliente` | S (ISO 8601 UTC) | cliente |
| `bloqueios` | L de M `{codigo: N, valor: N}`, ordenada por `codigo` | bloqueios |
| `data_atualizacao_bloqueios` | S (ISO 8601 UTC) | bloqueios |
| `version` | N | ambos (lock) |

**Regras de escrita**

- Item inexistente: cria só com os atributos do domínio e `version = 1` (condição: PK não existe)
- Item existente: leitura consistente → compara timestamp do domínio → atualiza só os atributos do domínio, com condição `version = lida`, incrementando
- Timestamp: grava só se o novo for **estritamente maior**; igual ou menor: descarta com sucesso (também torna reentregas idempotentes)
- `version` única na linha: os dois domínios disputam o mesmo lock; conflitos são absorvidos pelo retry interno
- Valores decimais convertidos a partir de texto, sem float
- Mensagem de bloqueios com zero itens grava lista vazia; cliente que nunca recebeu bloqueios fica sem o atributo

### Tabela `falhas_processamento`

- PK `id_mensagem` (S, messageId da SQS)
- Atributos: `dominio`, `fila`, `codigo_cliente` (se extraível), `corpo_original`, `tipo_erro` (permanente / transitório / esgotado), `mensagem_erro`, `tentativas`, `data_falha`
- Opcional: GSI por `codigo_cliente` + `data_falha`
- Gravada só na última tentativa (`ApproximateReceiveCount` = 3); a mensagem ainda é reportada como falha e segue para a DLQ
- Se a gravação falhar: fluxo normal (reporta falha; DLQ preserva a mensagem)

## 4. Contratos de entrada (1 mensagem = 1 cliente)

- **Cliente:** `codigo_cliente`, `numero_agencia`, `numero_conta`, `numero_dac`, `data_atualizacao`
- **Bloqueios:** `codigo_cliente`, pares `codigo_bloqueio_n` / `valor_bloqueio_n`, `data_atualizacao`

**Validações (todas são erro permanente)**

- JSON inválido
- GUID inválido
- Campo obrigatório ausente
- Tipo errado (inteiro, decimal)
- Timestamp inválido ou sem fuso (normaliza para UTC, formato fixo)
- Par incompleto
- Código de bloqueio duplicado (defensivo)
- Mais de 100 bloqueios

## 5. Classificação de resultados

- **Sucesso:** gravado, ou descartado por ser mais antigo/igual
- **Falha permanente:** validação → reporta falha → 3 recebimentos → DLQ
- **Falha transitória:** conflito de versão após 3 tentativas internas, throttling, rede → reporta falha
- **Última tentativa com falha:** grava em `falhas_processamento` e reporta falha
- Nunca retornar erro geral da função (o batch inteiro voltaria); sempre a lista de `itemIdentifier` com falha

## 6. Infraestrutura (Terraform)

- 2 filas + 2 DLQs (retenção 1 dia, `maxReceiveCount = 3`)
- 2 Lambdas (mesmo zip, `DOMAIN` por env, arquitetura parametrizável)
- 2 Event Source Mappings com `ReportBatchItemFailures`
- 2 tabelas DynamoDB (`cadastro_geral`, `falhas_processamento`)
- IAM com permissões mínimas por Lambda
- Visibility timeout ≥ 6× o timeout da Lambda
- Provider parametrizado (endpoint do emulador ou AWS real)

## 7. Estrutura do projeto (sugerida)

- `README.md` (passo a passo, resultado esperado, troubleshooting, plano B)
- `docker-compose.yml` (Floci, versão fixa)
- `Makefile` (`up`, `build`, `deploy`, `demo`, `test`, `logs`, `down`, `all`)
- `infra/` (Terraform)
- `src/` (`cmd/lambda`, `internal/domain`, `internal/repo`, `internal/handler`)
- `scripts/` (roteiro da demo)
- `samples/` (mensagens de exemplo)

## 8. Emulador e demo reproduzível

### Floci (principal)

- MIT, sem conta/token, porta 4566, mesmo padrão do LocalStack
- Projeto novo (2026): validar cedo com teste de fumaça
- Plano B: LocalStack com token Hobby (gratuito, não comercial)

### Para o avaliador

- Só Docker + Docker Compose; build, Terraform e demo em containers
- Versões fixas: Floci, Terraform, provider AWS, Go
- Portas configuráveis; espera de saúde do Floci antes do deploy
- Mensagens de erro claras no Makefile
- Arquitetura (amd64 × arm64) documentada
- Testar em ambiente limpo antes de enviar
- Vídeo/GIF como garantia

### Limitações a declarar

- Emuladores não validam IAM nem limites reais da AWS
- Ideal: deploy final em conta AWS real (opcional)

## 9. Plano por dia

### Dia 1: Fundação + teste de fumaça

- Estrutura, compose com Floci, Makefile inicial
- Terraform mínimo (fila + DLQ, Lambda, ESM com `ReportBatchItemFailures`, tabela)
- Lambda Go mínima que falha 1 mensagem de propósito
- **Saída:** runtime Go roda, falha parcial respeitada, DLQ recebe após os retries
- Se falhar: decidir o plano B no mesmo dia

### Dia 2: Infra completa + esqueleto

- Terraform completo (incluindo tabela de falhas e IAM)
- Estrutura Go, roteamento por `DOMAIN`, parsing e validações, interface do repositório
- **Saída:** `make all` sobe tudo e as Lambdas consomem (só log)

### Dia 3: Regras de negócio

- Escrita com leitura consistente + lock, timestamp por domínio
- Retry interno, classificação de erros
- Agrupamento por cliente com pool de workers
- Partial batch response completo
- Gravação na tabela de falhas na última tentativa
- **Saída:** fluxo ponta a ponta no emulador

### Dia 4: Testes

- Unitários (repositório mockado): parsing, timestamp, limite de 100, par incompleto, duplicados
- Integração no Floci: condition expressions, conflito de versão
- Concorrência entre domínios
- Esgotar retries → DLQ + tabela de falhas
- **Saída:** todos os cenários passam de forma repetível

### Dia 5: Demo e empacotamento

- Script de demo (envio, espera, impressão legível, resumo "N/N OK")
- README completo, vídeo/GIF
- Validação em ambiente limpo (amd64 e arm64), zip final

### Dias 6–7: Folga

- Entender cada linha e cada escolha (exigência do enunciado)
- Ensaio da apresentação: arquitetura → decisões → execução → limitações
- Opcionais: deploy em conta AWS real, logs estruturados

## 10. Cenários da demo

1. Cliente novo
2. Bloqueios antes do cliente
3. Lista de bloqueios substituída e ordenada
4. Mensagem mais antiga descartada
5. Domínios sem sobrescrita mútua
6. Concorrência entre domínios (conflito de versão + retry interno)
7. Batch com mensagem inválida (só ela falha)
8. Mais de 100 bloqueios (rejeitada)
9. Esgotar retries (DLQ + registro na tabela de falhas)

## 11. Riscos e gatilhos

- Teste de fumaça falha no Floci → plano B (LocalStack Hobby)
- Atraso no Dia 3 → cortar opcionais, manter lock simples
- Ambiente do avaliador diferente → vídeo + plano B no README
- Registrar as horas investidas desde o início (o enunciado cobra)

## 12. Preparação para a explicação oral

- Para cada decisão: justificativa + alternativa descartada
- Pontos centrais: timestamp por domínio, lock com `version`, lista substituída, agrupamento por cliente, tabela de falhas
- Revisar cada arquivo ao fim de cada dia
