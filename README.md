```markdown
# Minhas Finanças

Aplicação de linha de comando em Go para importar extratos CSV para PostgreSQL. Cada importação substitui os lançamentos anteriores pelos dados do arquivo escolhido.

## Pré-requisitos

- Go instalado
- Docker com Docker Compose
- Arquivo de extrato CSV na pasta do projeto, com cabeçalho `date,title,amount` e datas no formato `AAAA-MM-DD`

## Executar

1. Inicie manualmente o banco de dados:

	```sh
	docker compose up -d postgres
	```

2. Na pasta do projeto, inicie o programa:

	```sh
	go run .
	```

3. Use o menu numerado (também aceita os nomes das opções):

	- `1`: importar extrato; informe o mês/ano (`AAAA-MM`) e o nome do CSV na pasta do projeto. A importação substitui os lançamentos anteriores.
	- `2`: cadastrar categoria.
	- `3`: listar categorias.
	- `4`: excluir uma categoria pelo ID. Os vínculos de despesas são removidos; lançamentos existentes são preservados sem categoria.
	- `5`: vincular o nome de uma despesa do extrato a uma categoria, escolhida pelo ID. Cadastrar novamente uma despesa altera seu vínculo.
	- `6`: listar os nomes de despesas e suas categorias.
	- `7`: categorizar lançamentos conforme os nomes cadastrados. A comparação ignora maiúsculas/minúsculas; quando mais de um nome combina, usa o mais específico.
	- `8`: cadastrar despesa manual, informando primeiro o mês de referência (`AAAA-MM`), depois o dia, nome e valor. Esses lançamentos são preservados quando um novo CSV é importado.
	- `9`: mostrar o total geral das despesas.
	- `10`: mostrar o total das despesas por categoria, incluindo `Não categorizado` e um total de conferência.
	- `0`: sair.

Os relatórios consideram somente valores positivos; créditos, pagamentos e estornos com valor negativo não entram no total de despesas.

O mês/ano informado na importação fica salvo em `statement_month`. Os valores monetários podem usar vírgula decimal, por exemplo `34,76`.

4. Digite `sair` para encerrar.

A URL de conexão pode ser alterada em `.env` pela variável `DATABASE_URL`. Os dados do PostgreSQL ficam persistidos no volume Docker `pgdata`. Para parar o banco sem apagar os dados, use `docker compose down`.