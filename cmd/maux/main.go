package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Shopify/go-lua"

	"github.com/davecgh/go-spew/spew"
)

var describeKeys = []string{"summary", "homepage", "url", "mirror", "sha256"}

type manifest struct {
	filen   string
	lg      logger
	rt      *mauxRuntime
	details map[string]string
	hasrun  bool
}

func parseManifest(lg logger, filename string) (*manifest, error) {
	lg.Write(ldebug, `begin reading and parsing of manifest manifest_filename="%s"`, filename)
	m := &manifest{
		filen: filename,
		lg:    lg.WithPrefix("manifest"),
		rt:    newMauxLuaRuntime(lg),
	}
	m.rt.describeHandler = m.describe
	return m, nil
}

func (m *manifest) reset() error {
	m.details = map[string]string{}
	m.rt = newMauxLuaRuntime(m.lg)
	m.rt.describeHandler = m.describe
	m.hasrun = false
	return nil
}

func (m *manifest) run() error {
	if m.hasrun {
		return errors.New("manifest has already been run, call reset() and run again")
	}
	m.hasrun = true
	s := time.Now()
	m.lg.Write(ldebug, `beginning initial run of manifest file="%s"`, m.filen)
	err := lua.DoFile(m.rt.ls, m.filen)
	m.lg.Write(ldebug, "manifest run time %s", time.Since(s))
	return err
}

func (m *manifest) install() error {
	if !m.hasrun {
		return errors.New("manifest MUST be run to know how to install")
	}
	err := m.prepareEnvironment()
	if err != nil {
		return err
	}
	m.rt.ls.Global("install")
	return m.rt.ls.ProtectedCall(0, 0, 0)
}

func (m *manifest) prepareEnvironment() error {
	// download the source
	// TODO(nick): some retry logic perhaps?
	_, _, err := download(m.details["url"], downloadOptions{
		ignoreCache: true,
	})
	if err != nil {
		return err
	}
	// if sha256 != m.details["sha256"] {
	// 	return errors.New("shasum did not match. cancelling installation.")
	// }

	return nil
}

type downloadOptions struct {
	ignoreCache bool
}

func download(uri string, opts downloadOptions) (string, *os.File, error) {
	// var dest *os.File
	if opts.ignoreCache {

	} else {
		cachekey := normalizestr(uri)
		cacheDir := os.Getenv("HOME") + "/.maux/cache/" + cachekey
		if err := os.MkdirAll(cacheDir, 0666); err != nil {
			return "", nil, err
		}
		// var err error
		// dest, err = os.Create(cacheDir + "/download")
	}
	// cachek := normalizestr(uri)
	return "", nil, nil
}

func normalizestr(s string) string {
	out := []byte(s)
	for i, b := range out {
		// lowercase
		if b >= 'A' && b <= 'Z' {
			out[i] = byte(uint8(b) + uint8(32))
			continue
		}
		if (b < 'a' || b > 'z') &&
			(b < '0' || b > '9') {
			out[i] = '-'
		}
	}
	return string(out)
}

func (m *manifest) describe(description map[string]string) error {
	// TODO(nick): check description
	m.lg.Write(ldebug, `manifest has been described: "%s"`, description["summary"])
	m.details = description
	return nil
}

type mauxRuntime struct {
	ls              *lua.State
	lg              logger
	env             map[string]string
	describeHandler func(map[string]string) error
}

func newMauxLuaRuntime(lg logger) *mauxRuntime {
	mrt := &mauxRuntime{
		ls:  lua.NewState(),
		lg:  lg.WithPrefix("maux_runtime"),
		env: map[string]string{},
		describeHandler: func(_ map[string]string) error {
			return nil
		},
	}
	lua.OpenLibraries(mrt.ls)

	luaSetGlobalString(mrt.ls, "prefix", os.Getenv("HOME")+"/.maux")

	mrt.ls.Register("describe", func(l *lua.State) int {
		topidx := l.Top()
		mrt.lg.Write(ldebug, "describe function called with %d args", topidx)
		if l.IsTable(topidx) {
			desc := map[string]string{}
			for _, k := range describeKeys {
				l.Field(topidx, k)
				v, ok := l.ToString(l.Top())
				l.Pop(1)
				if !ok {
					continue
				}
				desc[k] = v
			}
			err := mrt.describeHandler(desc)
			if err != nil {
				luaError(l, err)
				return 0
			}
		} else {
			luaError(l, fmt.Errorf("invalid invokation of describe(), expected table, got %s", lua.TypeNameOf(l, topidx)))
		}
		return 0
	})
	mrt.ls.Register("exec", func(l *lua.State) int {
		n := l.Top()
		mrt.lg.Write(ldebug, "exec called with %d args", n)
		cmdName, args := "", []string{}
		for i := 1; i <= n; i++ {
			str, ok := l.ToString(i)
			if !ok {
				luaError(l, fmt.Errorf("invalid invocation of exec(), expected all string arguments, got %s at argument %d", lua.TypeNameOf(l, i), i))
				return 0
			}
			if i == 1 {
				cmdName = str
				continue
			}
			args = append(args, str)
		}
		mrt.lg.Write(ldebug, "exec: %s %s", cmdName, strings.Join(args, " "))
		spew.Dump(args)
		cmd := exec.Command(cmdName, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Start()
		if err != nil {
			luaError(l, err)
			return 0
		}

		err = cmd.Wait()
		if err != nil {
			luaError(l, err)
			return 0
		}
		return 0
	})
	mrt.ls.Register("env_set", func(l *lua.State) int {
		nargs := l.Top()
		if nargs < 2 {
			luaError(l, fmt.Errorf("invalid invocation of env_set(), expected 2 args, got %d", nargs))
			return 0
		}
		key, _ := l.ToString(1)
		if key == "" {
			// invocationError(l, "env_set", "expected non-empty string for argument 1, got", ...)
		}
		for i := 1; i <= 2; i++ {
			str, ok := l.ToString(i)
			if i == 1 {
				if !ok {
					luaError(l, fmt.Errorf("invalid invocation of env_set(), expected string for argument 1, got %s", lua.TypeNameOf(l, i)))
					return 0
				}
				key = str
				continue
			}
			if i == 2 {
				val = str
			}
		}
		mrt.env[key] = val
		return 0
	})
	mrt.ls.Register("env_get", func(l *lua.State) int {
		nargs := l.Top()
		if nargs != 1 {
			luaError(l, fmt.Errorf("invalid invocation of env_get(), expected 1 argument, got %d", nargs))
			return 0
		}
		k, ok := l.ToString(1)
		if !ok {
			invocationError(l, "env_get", "expected string, got %s", lua.TypeNameOf(l, 1))
			luaError(l, fmt.Errorf("invalid invocation of env_get(), expected string, got %s", lua.TypeNameOf(l, 1)))
		}
		if e, ok := mrt.env[k]; ok {
			l.PushString(e)
		}
		l.PushString(os.Getenv(k))
		return 1
	})

	return mrt
}

func invocationError(l *lua.State, fnname, problemFormat string, a ...interface{}) {}

func luaError(l *lua.State, err error) {
	l.PushString(fmt.Sprintf("error: %s", err))
	l.Error()
}

func luaSetGlobalString(l *lua.State, name, value string) {
	l.PushString(value)
	l.SetGlobal(name)
}

func _main() int {
	rootlog := &defLogger{
		prefix: "root",
		ll:     log.New(os.Stderr, "", 0),
	}

	m, err := parseManifest(rootlog, os.Args[1])
	if err != nil {
		rootlog.Write(lerror, "failed to parse manifest: %s", err)
		return 1
	}

	err = m.run()
	if err != nil {
		rootlog.Write(lerror, "failed to run manifest: %s", err)
		return 1
	}

	if err := os.MkdirAll("tmp", 0777); err != nil {
		rootlog.Write(lerror, err.Error())
		return 1
	}

	if err = os.RemoveAll("./tmp/dl"); err != nil {
		rootlog.Write(lerror, err.Error())
		return 1
	}

	f, err := os.Create("./tmp/dl")
	if err != nil {
		rootlog.Write(lerror, err.Error())
		return 1
	}

	res, err := http.Get(m.details["url"])
	if err != nil {
		rootlog.Write(lerror, err.Error())
		return 1
	}
	defer res.Body.Close()

	h := sha256.New()
	r := io.TeeReader(res.Body, h)

	_, err = io.Copy(f, r)
	if err != nil {
		rootlog.Write(lerror, err.Error())
		return 1
	}

	rootlog.Write(ldebug, "digest: %s", hex.EncodeToString(h.Sum(nil)))

	err = m.install()
	if err != nil {
		rootlog.Write(lerror, "failed to call install: %s", err)
		return 1
	}

	return 0
}

func downloadAndVerify(url, hash string) error {
	return nil
}

func main() {
	os.Exit(_main())
}
