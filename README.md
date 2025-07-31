# 🏆 Rinha de Backend 2025 - Payment Processing API

## 📋 Submissão

**Participante:** Samuel Silva  
**Tecnologias:** Go, Fiber, Redis, Haproxy, Docker  
**Repositório da Rinha:** [rinha-de-backend-2025](https://github.com/zanfranceschi/rinha-de-backend-2025)
**Repositório:** [rinha-de-backend-2025-go](https://github.com/samypng/rinha-de-backend-2025-go)  
**Contato:** samuelsilva1997@hotmail.com  

---

## 🚀 Sobre o Projeto

Esta é uma implementação para a Rinha de Backend 2025, focada em processamento de pagamentos com alta concorrência. A solução utiliza **streaming architecture** com **worker pools** para processar pagamentos em tempo real, **Redis** para filas e cache, e **Haproxy** para balanceamento de carga.

### 🎯 Características Principais

- ✅ **Streaming Payment Processing** - Processamento de pagamentos em tempo real com streams
- ✅ **Worker Pool Pattern** - Controle de concorrência com workers configuráveis
- ✅ **Queue** - Filas Redis para reprocessamento de pagamentos
- ✅ **Health Checks** - Verificação automática de saúde dos provedores de pagamento
- ✅ **Fallback Strategy** - Alternância automática entre provedores primário/secundário
- ✅ **Load Balancing** - Haproxy com round-robin entre 2 instâncias
- ✅ **Real-time Processing** - Streaming de pagamentos para baixa latência

---

## 🏗️ Arquitetura

```
Internet → Haproxy (port 9999) → [Backend1, Backend2] → Redis Streams
                                     ↓         ↓              ↓
                               Worker Pool Worker Pool   Stream Consumer
                                     ↓         ↓              ↓
                             Payment Processors ←──── Payment Stream
```

### 📊 Recursos

- **CPU Total:** 1.5 unidades
- **Memória Total:** 350MB
- **Instâncias Backend:** 2 (0.5 CPU, 50MB cada)
- **Haproxy:** 0.4 CPU, 50MB
- **Redis:** 0.1 CPU, 200MB
- **Workers por instância:** 50 (configurável)
- **Stream Buffer:** 100 pagamentos por instância
- **Processing Mode:** Real-time streaming

---

## 🔧 API Endpoints

### 📤 Processar Pagamento
```http
POST /payment
Content-Type: application/json

{
  "correlation_id": "payment-12345",
  "amount": 1000
}
```

**Resposta (201 Created):**
### 📊 Resumo de Pagamentos
```http
GET /payments-summary
```

**Resposta (200 OK):**
```json
{
    "default" : {
        "totalRequests": 43236,
        "totalAmount": 415542345.98
    },
    "fallback" : {
        "totalRequests": 423545,
        "totalAmount": 329347.34
    }
}
```

---

## ⚙️ Configuração

### 🌐 Variáveis de Ambiente

| Variável | Descrição | Padrão |
|----------|-----------|---------|
| `REDIS_ADDR` | Endereço do Redis | `redis:6379` |
| `PAYMENT_HOST_DEFAULT` | Provedor primário | `http://payment-processor-default:8080` |
| `PAYMENT_HOST_FALLBACK` | Provedor secundário | `http://payment-processor-fallback:8080` |
| `PAYMENT_WORKERS` | Número de workers | `10` |
| `PAYMENT_CHANNEL_SIZE` | Tamanho da fila | `100` |

### 🐳 Docker Compose

O projeto utiliza uma arquitetura multi-container com streaming:

- **backend1/backend2:** Instâncias da aplicação Go com stream processing
- **Haproxy:** Load balancer na porta 9999
- **redis:** Redis Streams, filas e cache de health checks
- **payment-processors:** Serviços externos simulados

---

## 🚀 Como Executar

### 📦 Pré-requisitos
- Docker e Docker Compose
- Git

### 🎬 Iniciando

```bash
# Clone o repositório
git clone https://github.com/samypng/rinha-de-backend-2025-go
cd rinha-de-backend-2025-go

# Inicie todos os serviços
./start.sh
```

### 🛑 Parando

```bash
# Pare todos os serviços
./stop.sh
```

### 🔗 URLs dos Serviços

- **API Principal:** http://localhost:9999
- **Haproxy:** http://localhost:9999
- **Backend 1:** http://localhost:8000 (interno)
- **Backend 2:** http://localhost:8001 (interno)
- **Payment Processor (Default):** http://localhost:8001
- **Payment Processor (Fallback):** http://localhost:8002
- **Redis:** localhost:6379 (interno)

---

## 🌊 Arquitetura de Streaming

### 📡 Como Funciona o Stream Processing

A aplicação utiliza **Redis Streams** para processamento de pagamentos em tempo real:

```
1. POST /payment → Queue no Redis Stream
2. Worker Pool consome Stream → Processa pagamento
3. Health Check & Fallback → Provider Selection
4. Payment Processing → External API calls
5. Stream Acknowledgment → Payment completed
```

### 🔄 Fluxo de Processamento

- **Ingestion:** Pagamentos são adicionados ao Redis Stream instantaneamente
- **Processing:** Workers consomem streams em paralelo com controle de concorrência
- **Resilience:** Falhas são automaticamente reprocessadas via stream groups

### 🧪 Teste Rápido

```bash
# Enviar pagamento para stream
curl -X POST http://localhost:9999/payment \
  -H "Content-Type: application/json" \
  -d '{
    "correlation_id": "stream-payment-001",
    "amount": 2500
  }'

# Verificar métricas de processamento
curl http://localhost:9999/payments-summary
```

---

## 📄 Licença

Este projeto é parte da Rinha de Backend 2025 e está disponível para fins educacionais.

---

## 🏆 Resultados

*Os resultados dos testes de carga serão atualizados após a execução oficial da Rinha de Backend 2025.*

Resultados Parcial

```json
{
  "participante": "samypng-go",
  "total_liquido": 331235.998870887,
  "total_bruto": 333265.3,
  "total_taxas": 21489.015,
  "descricao": "'total_liquido' é sua pontuação final. Equivale ao seu lucro. Fórmula: total_liquido + (total_liquido * p99.bonus) - (total_liquido * multa.porcentagem)",
  "p99": {
    "valor": "7.879218400000021ms",
    "bonus": 0.062415631999999575,
    "max_requests": "550",
    "descricao": "Fórmula para o bônus: max((11 - p99.valor) * 0.02, 0)"
  },
  "multa": {
    "porcentagem": 0,
    "total": 0,
    "composicao": {
      "total_inconsistencias": 0,
      "descricao": "Se 'total_inconsistencias' > 0, há multa de 35%."
    }
  },
  "lag": {
    "num_pagamentos_total": 16747,
    "num_pagamentos_solicitados": 16747,
    "lag": 0,
    "descricao": "Lag é a diferença entre a quantidade de solicitações de pagamentos vs o que foi realmente computado pelo backend. Mostra a perda de pagamentos possivelmente por estarem enfileirados."
  },
  "pagamentos_solicitados": {
    "qtd_sucesso": 16747,
    "qtd_falha": 0,
    "descricao": "'qtd_sucesso' foram requests bem sucedidos para 'POST /payments' e 'qtd_falha' os requests com erro."
  },
  "pagamentos_realizados_default": {
    "total_bruto": 285007.8,
    "num_pagamentos": 14322,
    "total_taxas": 14250.39,
    "descricao": "Informações do backend sobre solicitações de pagamento para o Payment Processor Default."
  },
  "pagamentos_realizados_fallback": {
    "total_bruto": 48257.5,
    "num_pagamentos": 2425,
    "total_taxas": 7238.625,
    "descricao": "Informações do backend sobre solicitações de pagamento para o Payment Processor Fallback."
  }
}
```