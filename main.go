package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mediator007/gitSercher/cmd"
	"github.com/spf13/cobra"
)

// -------------------- Структуры --------------------

// Задача поиска: репозиторий + ветка
type SearchTask struct {
	Repo   string
	Branch string
}

// Результат поиска
type SearchResult struct {
	Repo   string
	Branch string
	Line   string
}

// -------------------- Флаги CLI --------------------

var maxWorkers int       // Максимальное количество горутин
var spinner bool         // Показывать крутилку
var branchPattern string // паттерн веток для поиска
var searchDirs []string  // новые директории через флаг

// -------------------- Main --------------------

func main() {
	var rootCmd = &cobra.Command{
		Use:   "gitSrch [searchString] [path]",
		Short: "gitSrch helper",
		Long:  "CLI to search for a string in all branches of Git repositories",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			start := time.Now()
			searchString := args[0] // Строка поиска
			rootDir := "."          // Директория по умолчанию
			if len(args) > 1 {
				rootDir = args[1]
			}

			var dirsToScan []string

			if len(searchDirs) > 0 {
				// Если указаны поддиректории через --dirs, добавляем их к rootDir
				for _, d := range searchDirs {
					dirsToScan = append(dirsToScan, fmt.Sprintf("%s/%s", rootDir, d))
				}
			} else {
				// Иначе сканируем все поддиректории первого уровня
				entries, err := os.ReadDir(rootDir)
				if err != nil {
					fmt.Println("Error reading directory:", err)
					return
				}
				for _, e := range entries {
					if e.IsDir() {
						dirsToScan = append(dirsToScan, fmt.Sprintf("%s/%s", rootDir, e.Name()))
					}
				}
			}

			// Находим git-репозитории в dirsToScan
			var allRepos []string
			for _, dir := range dirsToScan {
				// Если сама директория - git репо, добавляем
				if _, err := os.Stat(dir + "/.git"); err == nil {
					allRepos = append(allRepos, dir)
					continue
				}

				repos, err := findGitReposNonRecursive(dir)
				if err != nil {
					fmt.Println("Error scanning", dir, ":", err)
					continue
				}
				allRepos = append(allRepos, repos...)
			}

			if len(allRepos) == 0 {
				fmt.Println("No git repositories found in specified directories")
				return
			}

			fmt.Printf("Found %d git repositories. Searching...\n", len(allRepos))

			// Выполняем поиск через пул горутин
			results := searchReposWithPool(allRepos, maxWorkers, searchString)

			// Выводим результаты
			for _, r := range results {
				fmt.Printf("[%s] [%s] %s\n", r.Repo, r.Branch, r.Line)
			}

			fmt.Printf("\nSearch finished. Total matches: %d\n", len(results))
			duration := time.Since(start)
			fmt.Printf("\nSearch finished in %s\n", duration)
		},
	}

	// Флаги
	rootCmd.PersistentFlags().IntVar(&maxWorkers, "max-workers", 10, "Maximum number of parallel searches")
	rootCmd.PersistentFlags().BoolVar(&spinner, "spinner", false, "Show spinner while searching")
	rootCmd.PersistentFlags().StringVar(&branchPattern, "branch-pattern", "", "Regexp pattern to filter branches (e.g. 'main|develop|feature/')")
	rootCmd.PersistentFlags().StringSliceVar(&searchDirs, "dirs", []string{"."}, "Comma-separated list of directories to search for git repositories")
	rootCmd.AddCommand(cmd.RecommendCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// -------------------- Функции --------------------

// findGitReposNonRecursive ищет git-репозитории только на первом уровне
func findGitReposNonRecursive(root string) ([]string, error) {
	var repos []string

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			gitPath := filepath.Join(root, entry.Name(), ".git")
			info, err := os.Stat(gitPath)
			if err == nil && info.IsDir() {
				repos = append(repos, filepath.Join(root, entry.Name()))
			}
		}
	}

	return repos, nil
}

// getGitBranches возвращает список всех веток репозитория
func getGitBranches(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "branch", "--all", "--format=%(refname:short)")
	cmd.Dir = repoPath

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git branch failed: %v, output: %s", err, out.String())
	}

	lines := strings.Split(out.String(), "\n")
	var branches []string
	for _, line := range lines {
		branch := strings.TrimSpace(line)
		if branch != "" {
			branches = append(branches, branch)
		}
	}

	return branches, nil
}

// filterBranches применяет паттерн к списку веток
func filterBranches(branches []string, pattern string) ([]string, error) {
	if pattern == "" {
		return branches, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid branch pattern: %v", err)
	}
	var filtered []string
	for _, b := range branches {
		if re.MatchString(b) {
			filtered = append(filtered, b)
		}
	}
	return filtered, nil
}

// searchStringInBranch ищет строку в указанной ветке репозитория
func searchStringInBranch(repoPath, branch, search string) ([]string, error) {
	cmd := exec.Command("git", "grep", "-n", search, branch, "--")
	cmd.Dir = repoPath

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		// git grep возвращает код 1, если совпадений нет — это не ошибка
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("git grep failed: %v, output: %s", err, out.String())
	}

	lines := strings.Split(out.String(), "\n")
	var results []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			results = append(results, line)
		}
	}

	return results, nil
}

func searchReposWithPool(repos []string, maxWorkers int, search string) []SearchResult {
	tasks := make(chan SearchTask)
	resultsCh := make(chan SearchResult)
	var wg sync.WaitGroup

	// Воркеры
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				lines, err := searchStringInBranch(task.Repo, task.Branch, search)
				if err != nil {
					fmt.Println("Error searching", task.Repo, task.Branch, err)
					continue
				}
				for _, line := range lines {
					resultsCh <- SearchResult{Repo: task.Repo, Branch: task.Branch, Line: line}
				}
			}
		}()
	}

	// Формируем задачи
	go func() {
		for _, repo := range repos {
			branches, err := getGitBranches(repo)
			if err != nil {
				fmt.Println("Error getting branches for", repo, err)
				continue
			}

			branches, err = filterBranches(branches, branchPattern)
			if err != nil {
				fmt.Println("Error filtering branches:", err)
				continue
			}

			for _, branch := range branches {
				tasks <- SearchTask{Repo: repo, Branch: branch}
			}
		}
		close(tasks)
	}()

	// Сбор результатов
	var results []SearchResult
	done := make(chan struct{})
	go func() {
		for r := range resultsCh {
			results = append(results, r)
		}
		close(done)
	}()

	// Крутилка
	var spinnerDone chan struct{}
	if spinner {
		spinnerDone = make(chan struct{})
		go runSpinner(spinnerDone)
	}

	wg.Wait()
	close(resultsCh)
	<-done

	if spinner {
		close(spinnerDone)
		fmt.Print("\r")
	}

	return results
}

// runSpinner выводит крутилку в консоль
func runSpinner(done chan struct{}) {
	spinChars := []rune{'|', '/', '-', '\\'}
	i := 0
	for {
		select {
		case <-done:
			return
		default:
			fmt.Printf("\r%c Searching...", spinChars[i%len(spinChars)])
			i++
			time.Sleep(100 * time.Millisecond)
		}
	}
}
