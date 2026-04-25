package ssh

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	cryptossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func askPass(msg string) string {
	fmt.Print(msg)
	pass, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	fmt.Println("")
	return strings.TrimSpace(string(pass))
}

func getPassphrase(ask bool) string {
	if ask {
		return askPass("Enter Private Key Passphrase: ")
	}
	return ""
}

func askIsHostTrusted(host string, key cryptossh.PublicKey) bool {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Unknown Host: %s \nFingerprint: %s \n", host, cryptossh.FingerprintSHA256(key))
	fmt.Print("Would you like to add it? type yes or no: ")
	answer, err := reader.ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	return strings.ToLower(strings.TrimSpace(answer)) == "yes"
}
