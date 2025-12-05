package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/cheggaaa/pb/v3"
	"github.com/fatih/color"
)

var (
	red    = color.New(color.FgRed).SprintFunc()
	green  = color.New(color.FgGreen).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()
	blue   = color.New(color.FgBlue).SprintFunc()
	white  = color.New(color.FgWhite).SprintFunc()

	domainRe = regexp.MustCompile(`^(?:https?://|android://|smtp://)?(?:www\.)?([^/]+)`)
	emailRe  = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	passRe   = regexp.MustCompile(`(?::|,|\s)([^,\s:]+)`)
	cpfRe    = regexp.MustCompile(`\b\d{11}\b`)
	cnpjRe   = regexp.MustCompile(`\b\d{14}\b`)
)

func clearScreen() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	_ = cmd.Run()
}

func getTxtFiles(directory string) ([]string, error) {
	var txtFiles []string
	err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".txt") {
			txtFiles = append(txtFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return txtFiles, nil
}

func calculateTotalSize(txtFiles []string) float64 {
	var totalSize int64
	for _, file := range txtFiles {
		fi, err := os.Stat(file)
		if err == nil {
			totalSize += fi.Size()
		}
	}
	return float64(totalSize) / (1024 * 1024 * 1024)
}

func newProgressBar(total int, label string) *pb.ProgressBar {
	bar := pb.StartNew(total)
	template := `{{string . "` + label + `"}} {{counters . }} {{bar . }} {{percent . }} {{etime . }} {{rtime . "ETA %s"}}`
	bar.SetTemplateString(template)
	return bar
}

func computeStats(txtFiles []string) (int, int) {
	totalLines := 0
	domains := make(map[string]struct{})
	var mu sync.Mutex

	totalLinesChan := make(chan int, len(txtFiles))
	domainsChan := make(chan string, 1000)

	bar := newProgressBar(len(txtFiles), "Processando")

	filesChan := make(chan string)
	numWorkers := runtime.NumCPU() * 2
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for file := range filesChan {
				f, err := os.Open(file)
				if err != nil {
					fmt.Println(red("Erro ao abrir:"), file, err)
					bar.Increment()
					continue
				}
				scanner := bufio.NewScanner(f)
				buf := make([]byte, 0, 1024*1024)
				scanner.Buffer(buf, 10*1024*1024)

				localLines := 0
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if line == "" {
						continue
					}
					localLines++
					match := domainRe.FindStringSubmatch(line)
					if len(match) > 1 {
						domain := strings.ToLower(match[1])
						domainsChan <- domain
					}
				}
				if err := scanner.Err(); err != nil {
					fmt.Println(red("Erro ao escanear:"), file, err)
				}
				_ = f.Close()
				totalLinesChan <- localLines
				bar.Increment()
			}
		}()
	}

	go func() {
		for _, file := range txtFiles {
			filesChan <- file
		}
		close(filesChan)
	}()

	go func() {
		wg.Wait()
		close(totalLinesChan)
		close(domainsChan)
		bar.Finish()
	}()

	for lines := range totalLinesChan {
		totalLines += lines
	}

	for domain := range domainsChan {
		mu.Lock()
		domains[domain] = struct{}{}
		mu.Unlock()
	}

	clearScreen()
	return totalLines, len(domains)
}

func ensureResultsDir() (string, error) {
	resultsDir := "resultados"
	if err := os.MkdirAll(resultsDir, os.ModePerm); err != nil {
		return "", err
	}
	return resultsDir, nil
}

func filterByDomain(txtFiles []string, domainInput string) int {
	domainInput = strings.ToLower(domainInput)
	resultsDir, err := ensureResultsDir()
	if err != nil {
		fmt.Println(red("Erro ao criar diretório:"), err)
		return 0
	}

	outputFile := filepath.Join(resultsDir, domainInput+".txt")
	output, err := os.Create(outputFile)
	if err != nil {
		fmt.Println(red("Erro ao criar arquivo:"), err)
		return 0
	}
	defer output.Close()

	matches := 0
	seen := make(map[string]struct{})
	var mu sync.Mutex

	bar := newProgressBar(len(txtFiles), "Filtrando")

	filesChan := make(chan string)
	numWorkers := runtime.NumCPU() * 2
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for file := range filesChan {
				f, err := os.Open(file)
				if err != nil {
					fmt.Println(red("Erro ao abrir:"), file, err)
					bar.Increment()
					continue
				}
				scanner := bufio.NewScanner(f)
				buf := make([]byte, 0, 1024*1024)
				scanner.Buffer(buf, 10*1024*1024)

				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if line == "" {
						continue
					}
					match := domainRe.FindStringSubmatch(line)
					if len(match) > 1 && strings.Contains(strings.ToLower(match[1]), domainInput) {
						mu.Lock()
						if _, ok := seen[line]; !ok {
							seen[line] = struct{}{}
							_, _ = output.WriteString(line + "\n")
							matches++
						}
						mu.Unlock()
					}
				}
				if err := scanner.Err(); err != nil {
					fmt.Println(red("Erro ao escanear:"), file, err)
				}
				_ = f.Close()
				bar.Increment()
			}
		}()
	}

	go func() {
		for _, file := range txtFiles {
			filesChan <- file
		}
		close(filesChan)
	}()

	wg.Wait()
	bar.Finish()
	clearScreen()
	return matches
}

func extractEmailsAndPasswords(txtFiles []string) int {
	resultsDir, err := ensureResultsDir()
	if err != nil {
		fmt.Println(red("Erro ao criar diretório:"), err)
		return 0
	}
	outputFile := filepath.Join(resultsDir, "emails_extracted.txt")
	output, err := os.Create(outputFile)
	if err != nil {
		fmt.Println(red("Erro ao criar arquivo:"), err)
		return 0
	}
	defer output.Close()

	matches := 0
	seen := make(map[string]struct{})
	var mu sync.Mutex

	bar := newProgressBar(len(txtFiles), "Extraindo Emails")

	filesChan := make(chan string)
	numWorkers := runtime.NumCPU() * 2
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for file := range filesChan {
				f, err := os.Open(file)
				if err != nil {
					fmt.Println(red("Erro ao abrir:"), file, err)
					bar.Increment()
					continue
				}
				scanner := bufio.NewScanner(f)
				buf := make([]byte, 0, 1024*1024)
				scanner.Buffer(buf, 10*1024*1024)

				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if line == "" {
						continue
					}
					emailMatch := emailRe.FindStringIndex(line)
					if emailMatch == nil {
						continue
					}
					email := line[emailMatch[0]:emailMatch[1]]
					rest := line[emailMatch[1]:]
					passMatch := passRe.FindStringSubmatch(rest)
					if len(passMatch) > 1 {
						password := passMatch[1]
						entry := email + ":" + password
						mu.Lock()
						if _, ok := seen[entry]; !ok {
							seen[entry] = struct{}{}
							_, _ = output.WriteString(entry + "\n")
							matches++
						}
						mu.Unlock()
					}
				}
				if err := scanner.Err(); err != nil {
					fmt.Println(red("Erro ao escanear:"), file, err)
				}
				_ = f.Close()
				bar.Increment()
			}
		}()
	}

	go func() {
		for _, file := range txtFiles {
			filesChan <- file
		}
		close(filesChan)
	}()

	wg.Wait()
	bar.Finish()
	clearScreen()
	return matches
}

func extractCNPJAndPasswords(txtFiles []string) int {
	resultsDir, err := ensureResultsDir()
	if err != nil {
		fmt.Println(red("Erro ao criar diretório:"), err)
		return 0
	}
	outputFile := filepath.Join(resultsDir, "cnpj_extracted.txt")
	output, err := os.Create(outputFile)
	if err != nil {
		fmt.Println(red("Erro ao criar arquivo:"), err)
		return 0
	}
	defer output.Close()

	matches := 0
	seen := make(map[string]struct{})
	var mu sync.Mutex

	bar := newProgressBar(len(txtFiles), "Extraindo CNPJ")

	filesChan := make(chan string)
	numWorkers := runtime.NumCPU() * 2
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for file := range filesChan {
				f, err := os.Open(file)
				if err != nil {
					fmt.Println(red("Erro ao abrir:"), file, err)
					bar.Increment()
					continue
				}
				scanner := bufio.NewScanner(f)
				buf := make([]byte, 0, 1024*1024)
				scanner.Buffer(buf, 10*1024*1024)

				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if line == "" {
						continue
					}
					cnpjMatch := cnpjRe.FindStringIndex(line)
					if cnpjMatch == nil {
						continue
					}
					cnpj := line[cnpjMatch[0]:cnpjMatch[1]]
					rest := line[cnpjMatch[1]:]
					passMatch := passRe.FindStringSubmatch(rest)
					if len(passMatch) > 1 {
						password := passMatch[1]
						entry := cnpj + ":" + password
						mu.Lock()
						if _, ok := seen[entry]; !ok {
							seen[entry] = struct{}{}
							_, _ = output.WriteString(entry + "\n")
							matches++
						}
						mu.Unlock()
					}
				}
				if err := scanner.Err(); err != nil {
					fmt.Println(red("Erro ao escanear:"), file, err)
				}
				_ = f.Close()
				bar.Increment()
			}
		}()
	}

	go func() {
		for _, file := range txtFiles {
			filesChan <- file
		}
		close(filesChan)
	}()

	wg.Wait()
	bar.Finish()
	clearScreen()
	return matches
}

func extractCPFAndPasswords(txtFiles []string) int {
	resultsDir, err := ensureResultsDir()
	if err != nil {
		fmt.Println(red("Erro ao criar diretório:"), err)
		return 0
	}
	outputFile := filepath.Join(resultsDir, "cpf_extracted.txt")
	output, err := os.Create(outputFile)
	if err != nil {
		fmt.Println(red("Erro ao criar arquivo:"), err)
		return 0
	}
	defer output.Close()

	matches := 0
	seen := make(map[string]struct{})
	var mu sync.Mutex

	bar := newProgressBar(len(txtFiles), "Extraindo CPF")

	filesChan := make(chan string)
	numWorkers := runtime.NumCPU() * 2
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for file := range filesChan {
				f, err := os.Open(file)
				if err != nil {
					fmt.Println(red("Erro ao abrir:"), file, err)
					bar.Increment()
					continue
				}
				scanner := bufio.NewScanner(f)
				buf := make([]byte, 0, 1024*1024)
				scanner.Buffer(buf, 10*1024*1024)

				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if line == "" {
						continue
					}
					cpfMatch := cpfRe.FindStringIndex(line)
					if cpfMatch == nil {
						continue
					}
					cpf := line[cpfMatch[0]:cpfMatch[1]]
					rest := line[cpfMatch[1]:]
					passMatch := passRe.FindStringSubmatch(rest)
					if len(passMatch) > 1 {
						password := passMatch[1]
						entry := cpf + ":" + password
						mu.Lock()
						if _, ok := seen[entry]; !ok {
							seen[entry] = struct{}{}
							_, _ = output.WriteString(entry + "\n")
							matches++
						}
						mu.Unlock()
					}
				}
				if err := scanner.Err(); err != nil {
					fmt.Println(red("Erro ao escanear:"), file, err)
				}
				_ = f.Close()
				bar.Increment()
			}
		}()
	}

	go func() {
		for _, file := range txtFiles {
			filesChan <- file
		}
		close(filesChan)
	}()

	wg.Wait()
	bar.Finish()
	clearScreen()
	return matches
}

func deleteResultsFolder() {
	resultsDir := "resultados"
	if err := os.RemoveAll(resultsDir); err == nil {
		fmt.Println(green("✔ Pasta 'resultados' excluída com sucesso."))
	} else {
		fmt.Println(red("✘ Erro ao excluir pasta 'resultados':"), err)
	}
}

func main() {
	clearScreen()
	reader := bufio.NewReader(os.Stdin)

	fmt.Print(green("Selecione o caminho da sua cloud: "))
	directory, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println(red("Erro ao ler entrada:"), err)
		return
	}
	directory = strings.TrimSpace(directory)
	if directory == "" {
		fmt.Println(red("Caminho inválido."))
		return
	}

	if _, err := os.Stat(directory); os.IsNotExist(err) {
		fmt.Println(red("O diretório não existe:"), directory)
		return
	}

	txtFiles, err := getTxtFiles(directory)
	if err != nil {
		fmt.Println(red("Erro ao buscar arquivos:"), err)
		return
	}
	if len(txtFiles) == 0 {
		fmt.Println(red("Nenhum arquivo TXT encontrado no diretório ou subdiretórios."))
		return
	}

	totalSizeGB := calculateTotalSize(txtFiles)

	var totalLines int
	var distinctDomains int
	var statsComputed bool

	for {
		clearScreen()
		fmt.Println(red("INTERPOL 777"))
		fmt.Println()
		fmt.Println(blue("INFORMAÇÕES DA CLOUD"))
		fmt.Println(blue("────────────────────"))
		fmt.Println(blue("• ") + green(fmt.Sprintf("Arquivos TXT encontrados: %d", len(txtFiles))))
		fmt.Println(blue("• ") + green(fmt.Sprintf("Tamanho total: %.2f GB", totalSizeGB)))
		fmt.Println()
		fmt.Println(blue("MENU"))
		fmt.Println(blue("────"))
		if statsComputed {
			fmt.Println(yellow("1.") + " Total de linhas: " + white(totalLines))
			fmt.Println(yellow("2.") + " Domínios distintos: " + white(distinctDomains))
		} else {
			fmt.Println(yellow("1.") + " Total de linhas: " + white("N/A"))
			fmt.Println(yellow("2.") + " Domínios distintos: " + white("N/A"))
		}
		fmt.Println(yellow("3.") + " Filtrar por domínio (ex: netflix)")
		fmt.Println(yellow("4.") + " Extrair emails e senhas")
		fmt.Println(yellow("5.") + " Extrair CNPJ e senhas")
		fmt.Println(yellow("6.") + " Extrair CPF e senhas")
		fmt.Println(yellow("7.") + " Excluir pasta 'resultados'")
		fmt.Println(yellow("8.") + " Sair")
		fmt.Println()
		fmt.Println(red("by: https://datasystem.network/ | Japonês da Federal"))
		fmt.Println()
		fmt.Print(green("Escolha uma opção (1-8): "))

		choiceLine, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println(red("Erro ao ler opção:"), err)
			return
		}
		choice := strings.TrimSpace(choiceLine)

		switch choice {
		case "1":
			if !statsComputed {
				totalLines, distinctDomains = computeStats(txtFiles)
				statsComputed = true
			}
			fmt.Println(blue("\nTotal de linhas: ") + yellow(totalLines))
			fmt.Print(green("Pressione Enter para continuar..."))
			reader.ReadString('\n')
		case "2":
			if !statsComputed {
				totalLines, distinctDomains = computeStats(txtFiles)
				statsComputed = true
			}
			fmt.Println(blue("\nDomínios distintos: ") + yellow(distinctDomains))
			fmt.Print(green("Pressione Enter para continuar..."))
			reader.ReadString('\n')
		case "3":
			fmt.Print(green("Digite o domínio a filtrar (ex: netflix): "))
			domainLine, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println(red("Erro ao ler domínio:"), err)
				fmt.Print(green("Pressione Enter para continuar..."))
				reader.ReadString('\n')
				continue
			}
			domain := strings.TrimSpace(domainLine)
			if domain == "" {
				fmt.Println(red("Domínio inválido."))
				fmt.Print(green("Pressione Enter para continuar..."))
				reader.ReadString('\n')
				continue
			}
			matches := filterByDomain(txtFiles, domain)
			fmt.Println(blue("\nTotal de ocorrências encontradas: ") + yellow(matches))
			if matches > 0 {
				fmt.Println(green(fmt.Sprintf("Resultados salvos em 'resultados/%s.txt'", domain)))
			} else {
				fmt.Println(red("Nenhuma ocorrência encontrada para este domínio."))
			}
			fmt.Print(green("Pressione Enter para voltar ao menu..."))
			reader.ReadString('\n')
		case "4":
			matches := extractEmailsAndPasswords(txtFiles)
			fmt.Println(blue("\nTotal de emails extraídos: ") + yellow(matches))
			if matches > 0 {
				fmt.Println(green("Resultados salvos em 'resultados/emails_extracted.txt'"))
			} else {
				fmt.Println(red("Nenhum email encontrado."))
			}
			fmt.Print(green("Pressione Enter para voltar ao menu..."))
			reader.ReadString('\n')
		case "5":
			matches := extractCNPJAndPasswords(txtFiles)
			fmt.Println(blue("\nTotal de CNPJ extraídos: ") + yellow(matches))
			if matches > 0 {
				fmt.Println(green("Resultados salvos em 'resultados/cnpj_extracted.txt'"))
			} else {
				fmt.Println(red("Nenhum CNPJ encontrado."))
			}
			fmt.Print(green("Pressione Enter para voltar ao menu..."))
			reader.ReadString('\n')
		case "6":
			matches := extractCPFAndPasswords(txtFiles)
			fmt.Println(blue("\nTotal de CPF extraídos: ") + yellow(matches))
			if matches > 0 {
				fmt.Println(green("Resultados salvos em 'resultados/cpf_extracted.txt'"))
			} else {
				fmt.Println(red("Nenhum CPF encontrado."))
			}
			fmt.Print(green("Pressione Enter para voltar ao menu..."))
			reader.ReadString('\n')
		case "7":
			deleteResultsFolder()
			fmt.Print(green("Pressione Enter para continuar..."))
			reader.ReadString('\n')
		case "8":
			clearScreen()
			fmt.Println(red("CLOUD DB"))
			fmt.Println(green("Encerrando programa..."))
			return
		default:
			fmt.Println(red("Opção inválida."))
			fmt.Print(green("Pressione Enter para continuar..."))
			reader.ReadString('\n')
		}
	}
}
