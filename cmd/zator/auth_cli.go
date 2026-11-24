package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AloofLibra/zator/internal/app"
	"golang.org/x/term"
)

const authUsage = `zator auth — управление доступом к Web UI

Использование:
  zator auth status
  zator auth set-password [-password PAROL]
  zator auth enable [-password PAROL]
  zator auth disable -y

Пароль: -password (виден в ps) или переменная ZATOR_PASSWORD.
`

func cmdAuth(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, authUsage)
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "status":
		return cmdAuthStatus(stdout, stderr)
	case "set-password":
		return cmdAuthSetPassword(rest, stdout, stderr)
	case "enable":
		return cmdAuthEnable(rest, stdout, stderr)
	case "disable":
		return cmdAuthDisable(rest, stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, authUsage)
		return 0
	default:
		fmt.Fprintf(stderr, "неизвестная подкоманда auth %q\n\n%s", sub, authUsage)
		return 2
	}
}

func cmdAuthStatus(stdout, stderr io.Writer) int {
	a, err := app.Load()
	if err != nil {
		fmt.Fprintf(stderr, "ошибка конфигурации: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "auth_enabled:  %t\n", a.Config.AuthEnabled)
	fmt.Fprintf(stdout, "password_set:  %t\n", a.Config.HasPassword())
	fmt.Fprintf(stdout, "session_set:   %t\n", a.Config.SessionToken != "")
	return 0
}

func cmdAuthSetPassword(args []string, stdout, stderr io.Writer) int {
	pass, err := resolvePasswordFlag(args, stderr)
	if err != nil {
		return 2
	}
	if pass == "" {
		pass, err = promptPassword(stderr, "Новый пароль: ", "Повтор пароля: ")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	a, err := app.Load()
	if err != nil {
		fmt.Fprintf(stderr, "ошибка конфигурации: %v\n", err)
		return 1
	}
	if err := a.SetPassword(pass); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := a.Save(); err != nil {
		fmt.Fprintf(stderr, "сохранение: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "Пароль установлен, проверка доступа включена.")
	fmt.Fprintln(stdout, "Перезапустите Web UI, если служба уже запущена.")
	return 0
}

func cmdAuthEnable(args []string, stdout, stderr io.Writer) int {
	pass, err := resolvePasswordFlag(args, stderr)
	if err != nil {
		return 2
	}
	a, err := app.Load()
	if err != nil {
		fmt.Fprintf(stderr, "ошибка конфигурации: %v\n", err)
		return 1
	}
	if !a.Config.HasPassword() {
		if pass == "" {
			pass, err = promptPassword(stderr, "Пароль: ", "Повтор пароля: ")
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
		if err := a.SetPassword(pass); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		if pass != "" {
			if err := a.SetPassword(pass); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		} else if err := a.EnableAuth(); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if err := a.Save(); err != nil {
		fmt.Fprintf(stderr, "сохранение: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "Проверка доступа включена.")
	return 0
}

func cmdAuthDisable(args []string, stdout, stderr io.Writer) int {
	yes := false
	for _, a := range args {
		switch a {
		case "-y", "--yes":
			yes = true
		case "-h", "--help":
			fmt.Fprintln(stdout, "zator auth disable -y — отключить проверку доступа к Web UI")
			return 0
		default:
			fmt.Fprintf(stderr, "неизвестный флаг %q\n", a)
			return 2
		}
	}
	if !yes {
		fmt.Fprintln(stderr, "будет отключена проверка доступа; повторите с флагом -y")
		return 1
	}
	a, err := app.Load()
	if err != nil {
		fmt.Fprintf(stderr, "ошибка конфигурации: %v\n", err)
		return 1
	}
	a.ClearAuth()
	if err := a.Save(); err != nil {
		fmt.Fprintf(stderr, "сохранение: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "Проверка доступа отключена (пароль сохранён; включите: zator auth enable).")
	return 0
}

func resolvePasswordFlag(args []string, stderr io.Writer) (string, error) {
	pass := ""
	fromFlag := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-password" || a == "--password":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "нужно значение после -password")
				return "", fmt.Errorf("password flag")
			}
			i++
			pass = args[i]
			fromFlag = true
		case strings.HasPrefix(a, "-password="):
			pass = strings.TrimPrefix(a, "-password=")
			fromFlag = true
		case strings.HasPrefix(a, "--password="):
			pass = strings.TrimPrefix(a, "--password=")
			fromFlag = true
		case a == "-h" || a == "--help":
			return "", fmt.Errorf("help")
		default:
			fmt.Fprintf(stderr, "неизвестный аргумент %q\n", a)
			return "", fmt.Errorf("bad arg")
		}
	}
	if pass == "" {
		pass = strings.TrimSpace(os.Getenv("ZATOR_PASSWORD"))
	}
	if fromFlag && pass != "" {
		fmt.Fprintln(stderr, "предупреждение: -password виден в списке процессов (ps); предпочтительнее ZATOR_PASSWORD")
	}
	return pass, nil
}

// promptPassword запрашивает пароль дважды. Без TTY — ошибка (нужен -password).
func promptPassword(stderr io.Writer, first, second string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("нет TTY: задайте пароль флагом -password или используйте -no-auth")
	}
	fmt.Fprint(stderr, first)
	b1, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(stderr, second)
	b2, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(stderr)
	if err != nil {
		return "", err
	}
	p1, p2 := strings.TrimSpace(string(b1)), strings.TrimSpace(string(b2))
	if p1 != p2 {
		return "", fmt.Errorf("пароли не совпадают")
	}
	if p1 == "" {
		return "", fmt.Errorf("пустой пароль")
	}
	return p1, nil
}

