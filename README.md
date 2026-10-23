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

	- `1 - Cadastros`: importar extrato (`1`), cadastrar categoria (`2`), excluir categoria (`4`), vincular despesa a categoria (`5`), categorizar lançamentos (`7`) e cadastrar despesa manual (`8`).
	- `2 - Consultas`: listar categorias (`3`) e listar despesas e categorias (`6`).
	- `3 - Relatórios`: mostrar total geral de despesas (`9`) e despesas por categoria (`10`).
	- `4 - Ferramentas`: exportar todos os dados para um CSV (`1`) ou importar um CSV de backup (`2`). Informe apenas o nome do arquivo, que deve estar na pasta do projeto. A restauração preserva IDs, categorias, vínculos e lançamentos, e só é permitida quando as tabelas do banco de destino estão vazias. A operação é integral: se algum registro for inválido, nada é gravado.
	- `0`: voltar nos submenus ou sair no menu principal.

Os relatórios consideram somente valores positivos; créditos, pagamentos e estornos com valor negativo não entram no total de despesas.

O mês/ano informado na importação fica salvo em `statement_month`. Os valores monetários podem usar vírgula decimal, por exemplo `34,76`.

4. Digite `sair` para encerrar.

A URL de conexão pode ser alterada em `.env` pela variável `DATABASE_URL`. Os dados do PostgreSQL ficam persistidos no volume Docker `pgdata`. Para parar o banco sem apagar os dados, use `docker compose down`.