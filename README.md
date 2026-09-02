```markdown
# 📊 Gerenciador de Finanças Pessoais - Nubank

Aplicação CLI desenvolvida em Golang para importar, armazenar e categorizar extratos de cartão de crédito do Nubank em formato CSV, utilizando PostgreSQL e Docker.

---

## 🚀 Tecnologias Utilizadas

- **Go 1.26** (para o binário da aplicação)
- **PostgreSQL 16** (banco de dados relacional com healthcheck integrado)
- **pgx/v5** (driver de alta performance para conexão com o Postgres)
- **Docker & Docker Compose** (orquestração de containers)

---

## 🛠️ Pré-requisitos

Certifique-se de ter instalado na sua máquina:
- [Docker](https://www.docker.com/) e Docker Compose
- [Git](https://git-scm.com/) (opcional, para clonagem)

---

## 📂 Estrutura do Projeto

```text
.
├── main.go               # Código fonte da aplicação Golang (CLI + Banco de Dados)
├── Dockerfile            # Multi-stage build para Go 1.26 com alpine
├── docker-compose.yml    # Orquestração dos serviços (Postgres + App)
├── go.mod                # Dependências do Go
├── go.sum                # Checksum das dependências
└── *.csv                 # Extratos do Nubank (ex: Nubank_2026-09-12.csv)

```

---

## 📋 Passo a Passo para Executar o Projeto

### 1. Organizar os arquivos

Certifique-se de que todos os arquivos (`main.go`, `Dockerfile`, `docker-compose.yml`, `go.mod`, `go.sum`) estão na mesma pasta do projeto e que o seu arquivo de extrato em `.csv` do Nubank também está na raiz do diretório.

### 2. Construir e subir os containers

Abra o terminal na pasta do projeto e execute o comando abaixo para realizar o build da aplicação e iniciar o banco de dados em segundo plano:

```bash
docker compose up -d --build

```

*O Docker iniciará o PostgreSQL e aguardará o banco estar pronto e aceitando conexões antes de inicializar o container da aplicação Go.*

### 3. Acessar o terminal interativo da aplicação

Para interagir com a CLI (digitar os comandos), conecte-se ao container em execução:

```bash
docker attach golang_finances

```

*(Caso queira sair da sessão sem desligar o container, pressione `Ctrl + P` seguido de `Ctrl + Q`).*

### 4. Utilizar os comandos no terminal

Assim que a aplicação iniciar, você verá o prompt interativo `>`. Os comandos disponíveis são:

* **`/help`**: Exibe a lista completa de comandos e suas descrições.
```text
> /help

```


* **`/import`**: Lê automaticamente todos os arquivos `.csv` presentes no diretório e registra os novos gastos no banco de dados (evitando duplicatas).
```text
> /import
Importação concluída. 68 novas transações registradas.

```


* **`/categoria "<titulo>" <nome_categoria>`**: Cria ou atualiza uma regra de histórico. Sempre que o título do gasto contiver o texto especificado entre aspas, ele será categorizado automaticamente.
```text
> /categoria "Mercado Lizza" Mercado
> /categoria "Auto Post Central Li" Transporte

```


* **`/classificar`**: Aplica as regras cadastradas às transações pendentes ("Não Classificado") e exibe os itens que ainda não possuem regra correspondente.
```text
> /classificar

```



---

## 🛑 Como Parar os Serviços

Para pausar os containers preservando os dados salvos no banco:

```bash
docker compose down

```

Para parar os containers e apagar todos os dados do banco (limpando o volume persistente):

```bash
docker compose down -v

```

```

```