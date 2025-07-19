# 🏆 Rinha de Backend 2025 - Payment Processing API

## 📋 Submissão

**Participante:** Samuel Silva  
**Tecnologias:** Go, Fiber, Redis, PostgreSQL, Nginx, Docker  
**Repositório:** [rinha-de-backend-2025-go](https://github.com/user/rinha-de-backend-2025-go)  
**Contato:** samuel.silva@email.com  

---

## 🚀 Sobre o Projeto

Esta é uma implementação para a Rinha de Backend 2025, focada em processamento de pagamentos com alta concorrência. A solução utiliza **worker pools** para controlar a concorrência, **Redis** para filas de retry, e **nginx** para balanceamento de carga.

### 🎯 Características Principais

- ✅ **Worker Pool Pattern** - Controle de concorrência com workers configuráveis
- ✅ **Queue** - Filas Redis para reprocessamento de pagamentos
- ✅ **Health Checks** - Verificação automática de saúde dos provedores de pagamento
- ✅ **Fallback Strategy** - Alternância automática entre provedores primário/secundário
- ✅ **Load Balancing** - Nginx com round-robin entre 2 instâncias
- ✅ **Graceful Shutdown** - Finalização limpa dos workers

---

## 🏗️ Arquitetura

```
Internet → Nginx (port 9999) → [Backend1, Backend2] → Redis
                                     ↓         ↓
                               Worker Pool Worker Pool
                                     ↓         ↓
                             Payment Processors
```

### 📊 Recursos

- **CPU Total:** 1.5 unidades
- **Memória Total:** 250MB
- **Instâncias Backend:** 2 (0.5 CPU, 50MB cada)
- **Nginx:** 0.4 CPU, 50MB
- **Redis:** 0.1 CPU, 200MB
- **Workers por instância:** 150 (configurável)
- **Queue Size:** 1000 pagamentos por instância

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

O projeto utiliza uma arquitetura multi-container:

- **backend1/backend2:** Instâncias da aplicação Go
- **nginx:** Load balancer na porta 9999
- **redis:** Fila e cache de health checks
- **payment-processors:** Serviços externos simulados

---

## 🚀 Como Executar

### 📦 Pré-requisitos
- Docker e Docker Compose
- Git

### 🎬 Iniciando

```bash
# Clone o repositório
git clone https://github.com/user/rinha-de-backend-2025-go
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
- **Nginx:** http://localhost:9999
- **Backend 1:** http://localhost:8000 (interno)
- **Backend 2:** http://localhost:8001 (interno)
- **Payment Processor (Default):** http://localhost:8001
- **Payment Processor (Fallback):** http://localhost:8002
- **Redis:** localhost:6379 (interno)

---

## 📄 Licença

Este projeto é parte da Rinha de Backend 2025 e está disponível para fins educacionais.

---

## 🏆 Resultados

*Os resultados dos testes de carga serão atualizados após a execução oficial da Rinha de Backend 2025.*
