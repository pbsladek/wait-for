package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

var backendHelp = map[string]string{
	"http":       "HTTP:\n  waitfor http [flags] URL\n  --status 200|2xx --method GET --body text --body-file path --body-contains text --body-matches regex --jsonpath expr --header Key=Value --bearer-token TOKEN --basic-user USER --basic-password PASS --client-cert path --client-key path --insecure --no-follow-redirects\n",
	"tcp":        "TCP:\n  waitfor tcp HOST:PORT\n",
	"unix":       "Unix Socket:\n  waitfor unix PATH\n",
	"ports":      "Ports:\n  waitfor ports HOST --range START-END [--any|--all]\n",
	"tls":        "TLS:\n  waitfor tls HOST:PORT [--servername name] [--valid-for duration] [--ca-file path]\n",
	"ssh":        "SSH:\n  waitfor ssh HOST:PORT [--banner-contains text] [--user name --password value --host-key-sha256 fp]\n",
	"s3":         "S3:\n  waitfor s3 s3://bucket[/key] [--exists] [--metadata Key=Value] [--contains text] [--endpoint-url URL] [--virtual-hosted-style] [--region name]\n",
	"dns":        "DNS:\n  waitfor dns HOST [--type A|AAAA|CNAME|TXT|ANY|MX|SRV|NS|CAA|HTTPS|SVCB] [--resolver system|wire] [--contains text] [--equals value] [--min-count N] [--absent]\n",
	"docker":     "Docker:\n  waitfor docker CONTAINER [--status state] [--health state]\n",
	"process":    "Process:\n  waitfor process (--pid PID | --name NAME) [--running|--stopped]\n",
	"systemd":    "Systemd:\n  waitfor systemd UNIT [--active|--inactive|--failed]\n",
	"launchd":    "Launchd:\n  waitfor launchd LABEL [--running|--loaded]\n",
	"pidfile":    "PID file:\n  waitfor pidfile PATH [--running|--stopped]\n",
	"lockfile":   "Lockfile:\n  waitfor lockfile PATH [--absent|--present] [--older-than DURATION]\n",
	"permission": "Permission:\n  waitfor permission PATH [--mode 0644] [--uid UID|--user UID] [--gid GID|--group GID] [--type any|file|dir|symlink]\n",
	"checksum":   "Checksum:\n  waitfor checksum PATH --equals [ALGORITHM:]HEX [--algorithm auto|sha256|sha512|sha1]\n",
	"archive":    "Archive:\n  waitfor archive PATH (--contains MEMBER | --matches GLOB) [--format auto|tar|tgz|zip]\n",
	"cosign":     "Cosign:\n  waitfor cosign IMAGE [--key KEY] [--certificate CERT] [--certificate-identity ID] [--certificate-oidc-issuer URL]\n  waitfor cosign --blob FILE --signature SIG [--key KEY] [--certificate CERT]\n",
	"ntp":        "NTP:\n  waitfor ntp HOST[:PORT] [--max-offset DURATION] [--timeout DURATION]\n",
	"icmp":       "ICMP:\n  waitfor icmp HOST [--count N] [--timeout DURATION]\n",
	"grpc":       "gRPC:\n  waitfor grpc ADDRESS [--service NAME] [--method /pkg.Service/Method] [--reflect] [--status SERVING|NOT_SERVING|UNKNOWN|SERVICE_UNKNOWN] [--tls] [--timeout DURATION]\n",
	"websocket":  "WebSocket:\n  waitfor websocket ws://HOST/PATH [--send TEXT] [--contains TEXT|--matches REGEX] [--ping] [--expect-close-code N] [--read-timeout DURATION] [--header Key=Value] [--timeout DURATION]\n",
	"exec":       "Exec:\n  waitfor exec [flags] -- COMMAND [ARGS...]\n  --exit-code 0 --output-contains text --jsonpath expr --cwd path --env KEY=VALUE --max-output-bytes N\n",
	"file":       "File:\n  waitfor file PATH [--exists|--deleted|--nonempty] [--contains text]\n",
	"glob":       "Glob:\n  waitfor glob PATTERN [--min-count N] [--max-count N] [--absent]\n",
	"log":        "Log:\n  waitfor log PATH [--contains text|--matches regex|--jsonpath expr] [--exclude regex] [--from-start|--tail N] [--min-matches N]\n",
	"k8s":        "Kubernetes:\n  waitfor k8s RESOURCE [--condition type|--jsonpath expr|--for ready|rollout|complete] [--selector labels] [--all] [--namespace ns] [--kubeconfig path]\n",
}

func isBackendHelpCommand(args []string) bool {
	return len(args) > 0 && args[0] == "help" && len(args) > 1
}

func runBackendHelp(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: waitfor help BACKEND")
	}
	name := strings.ToLower(args[0])
	text, ok := backendHelp[name]
	if !ok {
		return fmt.Errorf("unknown backend %q", args[0])
	}
	_, _ = io.WriteString(stdout, text)
	return nil
}

func isCompletionCommand(args []string) bool {
	return len(args) > 0 && args[0] == "completion"
}

func runCompletion(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: waitfor completion bash|zsh|fish")
	}
	backends := backendNames()
	switch args[0] {
	case "bash":
		_, _ = fmt.Fprintf(stdout, "complete -W %q waitfor\n", strings.Join(append([]string{"doctor", "help", "completion"}, backends...), " "))
	case "zsh":
		_, _ = fmt.Fprintf(stdout, "#compdef waitfor\n_arguments '1:command:(%s)'\n", strings.Join(append([]string{"doctor", "help", "completion"}, backends...), " "))
	case "fish":
		for _, item := range append([]string{"doctor", "help", "completion"}, backends...) {
			_, _ = fmt.Fprintf(stdout, "complete -c waitfor -f -a %s\n", item)
		}
	default:
		return fmt.Errorf("unsupported shell %q", args[0])
	}
	return nil
}

func backendNames() []string {
	names := make([]string, 0, len(backendParsers))
	for name := range backendParsers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
