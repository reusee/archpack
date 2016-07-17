package main

import (
	"bufio"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	pt = fmt.Printf

	out string
	bin string
)

func init() {
	flag.StringVar(&out, "out", "archpack_out", "output directory")
	flag.StringVar(&bin, "bin", "/usr/bin/bash", "output binary")
	flag.Parse()
}

func main() {
	ce(os.MkdirAll(out, 0755), "out")

	// basic dirs
	ce(os.MkdirAll(out+"/etc", 0755), "etc")
	os.Symlink("usr/lib", out+"/lib64")

	// machine id
	idBytes := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, idBytes)
	ce(err, "read 16 bytes")
	err = ioutil.WriteFile(out+`/etc/machine-id`,
		[]byte(fmt.Sprintf("%x", idBytes)), 0755)
	ce(err, "write machine id")

	// extra files
	for _, f := range []string{
		"/etc/group",
		"/etc/passwd",
		"/usr/bin/getent",

		"/usr/lib/ld-2.23.so",
		"/lib64/ld-linux-x86-64.so.2",
	} {
		ce(exec.Command("cp", "-ar", "--parents",
			f, out).Run(), "cp")
	}

	// binary file
	ce(exec.Command("cp", "-a", "--parents",
		bin, out).Run(), "cp")

	cmd := exec.Command("sysdig", "user.name=postgres",
		"and", "evt.type=open",
		"and", "evt.dir='<'",
		"-p", `%proc.name|%fd.name`,
	)
	stdout, err := cmd.StdoutPipe()
	ce(err, "get stdout")
	ce(cmd.Start(), "start")

	reader := bufio.NewReader(stdout)
	filePaths := make(map[string]bool)
loop_lines:
	for {
		line, err := reader.ReadString('\n')
		ce(err, "read line")
		parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
		proc := parts[0]
		filePath := parts[1]

		// ignore dirs
		for _, d := range []string{
			"/postgres/",
			"/sys/",
			"/dev/",
			"/proc/",
			"/tmp/",
		} {
			if strings.HasPrefix(filePath, d) {
				continue loop_lines
			}
		}
		// ignore files
		for _, f := range []string{
			"/etc/resolv.conf",
			"/etc/ld.so.cache",
		} {
			if filePath == f {
				continue loop_lines
			}
		}
		// dedup
		if _, ok := filePaths[filePath]; ok {
			continue
		}
		filePaths[filePath] = true

		// existence
		stat, err := os.Lstat(filePath)
		if os.IsNotExist(err) {
			continue
		}
		ce(err, "stat")

		pt("%s %s\n", proc, filePath)

		// copy
		if stat.Mode()&os.ModeSymlink != 0 { // symlink
			dest, err := os.Readlink(filePath)
			ce(err, "get dest")
			if stat.IsDir() {
				ce(os.Symlink(out+filePath, dest), "symlink")
			} else {
				ce(exec.Command("cp", "-a", "--parents",
					filePath, out).Run(), "cp")
				dest, err = filepath.EvalSymlinks(filePath)
				ce(err, "eval symlink")
				dest, err = filepath.Abs(dest)
				ce(err, "get abs dest")
				ce(exec.Command("cp", "-a", "--parents",
					dest, out).Run(), "cp")
			}
		} else {
			if stat.IsDir() {
				ce(os.MkdirAll(out+filePath, 0755), "mkdir")
			} else {
				ce(exec.Command("cp", "-a", "--parents",
					filePath, out).Run(), "cp")
			}
		}

	}

}
