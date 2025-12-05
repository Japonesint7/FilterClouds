# Cloud TXT Analyzer

Ferramenta em Go para análise de grandes volumes de arquivos `.txt` armazenados em pastas locais (ex: “clouds” sincronizadas).  
O foco é facilitar auditorias e tratamentos de dados **em ambientes autorizados**, extraindo estatísticas, domínios e possíveis credenciais presentes nos arquivos.

> ⚠️ **Aviso importante**  
> Esta ferramenta deve ser utilizada apenas em dados próprios ou em ambientes onde você possua autorização formal (auditorias, segurança, compliance, etc.).  
> O uso indevido em bases de terceiros, vazamentos ou qualquer contexto não autorizado pode ser ilegal.

---

## Funcionalidades

- Varredura recursiva de diretórios em busca de arquivos `.txt`
- Cálculo de:
  - Quantidade total de arquivos encontrados
  - Tamanho total aproximado em GB
  - Total de linhas processadas
  - Quantidade de domínios distintos detectados
- Filtro por domínio:
  - Busca linhas que contenham determinado domínio (ex.: `netflix`, `spotify`) e salva em um arquivo separado
- Extração de possíveis credenciais:
  - `email:senha`
  - `CNPJ:senha`
  - `CPF:senha`
- Evita duplicados dentro dos resultados gerados
- Processamento paralelo (multithread), utilizando múltiplos cores da máquina
- Feedback visual com barra de progresso no terminal
- Menu interativo em modo texto

---

## Tecnologias utilizadas

- [Go](https://go.dev/)
- [cheggaaa/pb](https://github.com/cheggaaa/pb) para barra de progresso
- [fatih/color](https://github.com/fatih/color) para saída colorida no terminal

---

## Pré-requisitos

- Go 1.20+ instalado
- Ambiente com acesso aos arquivos `.txt` que serão analisados
- Permissão/autorização para processar os dados

---

## Instalação

```bash
git clone https://github.com/seu-usuario/seu-repositorio.git
cd seu-repositorio

go mod tidy
go build -o cloudmain.go
