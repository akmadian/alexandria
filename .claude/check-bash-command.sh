#!/usr/bin/env bash
# PreToolUse hook for Bash commands.
#
# Logic:
#   1. Split compound commands into subcommands (&&, ||, ;, |, $())
#   2. Check each subcommand against DENY patterns → prompt user ("ask")
#   3. If no deny match → auto-approve
#
# This means: all bash is auto-approved UNLESS a subcommand matches a deny pattern,
# in which case the user gets prompted (and can still approve).

set -euo pipefail

INPUT=$(cat)

RESULT=$(python3 -c "
import json, re, sys

data = json.load(sys.stdin)
command = data.get('input', {}).get('command', '')

if not command.strip():
    sys.exit(0)

# ── Deny patterns (regex, checked against each subcommand) ──
DENY_PATTERNS = [
    # Destructive filesystem
    (r'rm\s+.*-[rR]', 'rm -r/rf'),
    (r'\bsudo\s', 'sudo'),
    (r'chmod\s+777', 'chmod 777'),
    (r'chmod\s+-R\s', 'recursive chmod'),
    (r'chown\s+-R\s', 'recursive chown'),
    (r'\bmkfs', 'mkfs'),
    (r'\bdd\s+if=', 'dd'),
    (r'\bshred\s', 'shred'),
    (r'\btruncate\s', 'truncate'),
    (r'\bwipefs\s', 'wipefs'),
    (r'diskutil\s+erase', 'diskutil erase'),
    (r'\bfdisk\s', 'fdisk'),
    (r'>\s*/(?!tmp|private/tmp|dev/null)', 'truncate root file'),

    # Destructive git
    (r'git\s+push\s+.*--force', 'git push --force'),
    (r'git\s+push\b.*\s-f\b', 'git push -f'),
    (r'git\s+reset\s+--hard', 'git reset --hard'),
    (r'git\s+clean\s+-[a-z]*f', 'git clean -f'),
    # git branch -D handled separately (case-sensitive)
    (r'git\s+reflog\s+expire', 'git reflog expire'),
    (r'git\s+gc\s+--prune', 'git gc --prune'),
    (r'filter-branch', 'filter-branch'),
    (r'filter-repo', 'filter-repo'),
    (r'git\s+checkout\s+--\s+\.', 'git checkout -- .'),
    (r'git\s+restore\s+\.', 'git restore .'),
    (r'git\s+stash\s+(drop|clear)', 'git stash drop/clear'),

    # Destructive GitHub
    (r'gh\s+repo\s+delete', 'gh repo delete'),
    (r'gh\s+repo\s+archive', 'gh repo archive'),

    # Destructive brew
    (r'brew\s+(uninstall|remove)\s', 'brew uninstall'),

    # 1Password mutations
    (r'op\s+(item|document)\s+(create|edit|delete)', 'op vault mutation'),
    (r'op\s+vault\s+delete', 'op vault delete'),

    # Package manager dangers
    (r'pip\s+uninstall', 'pip uninstall'),
    (r'npm\s+publish', 'npm publish'),
    (r'npm\s+unpublish', 'npm unpublish'),
    (r'npm\s+install\s+-g', 'npm install -g'),

    # Cloud destructive ops
    (r'gcloud\s+.*\bdelete\b', 'gcloud delete'),
    (r'gcloud\s+projects\s+delete', 'gcloud projects delete'),
    (r'aws\s+\S+\s+(delete-|terminate-)', 'aws delete/terminate'),
    (r'aws\s+s3\s+r[bm]\s', 'aws s3 rm/rb'),
    (r'terraform\s+destroy', 'terraform destroy'),
    (r'terraform\s+apply\s+.*-auto-approve', 'terraform apply -auto-approve'),
    (r'pulumi\s+destroy', 'pulumi destroy'),
    (r'kubectl\s+delete\s+(namespace|ns\b|--all|-A)', 'kubectl delete namespace/all'),
    (r'kubectl\s+drain\s', 'kubectl drain'),
    (r'heroku\s+apps:destroy', 'heroku destroy'),
    (r'heroku\s+pg:reset', 'heroku pg:reset'),
    (r'vercel\s+(remove|rm)\s', 'vercel remove'),
    (r'fly\s+(apps|volumes)\s+destroy', 'fly destroy'),

    # Docker destructive
    (r'docker\s+system\s+prune', 'docker system prune'),
    (r'docker\s+volume\s+(rm|prune)', 'docker volume rm/prune'),
    (r'docker\s+container\s+prune', 'docker container prune'),
    (r'docker\s+compose\s+down\s.*-v', 'docker compose down -v'),
    (r'docker\s+rm\s+-f', 'docker rm -f'),

    # Database destructive (case-insensitive)
    (r'DROP\s+(DATABASE|TABLE|SCHEMA|USER)', 'SQL DROP'),
    (r'TRUNCATE\s+TABLE', 'SQL TRUNCATE'),
    (r'DELETE\s+FROM\s+\S+\s*$', 'DELETE FROM without WHERE'),
    (r'FLUSHALL', 'Redis FLUSHALL'),
    (r'FLUSHDB', 'Redis FLUSHDB'),
    (r'\bdropdb\s', 'dropdb'),
    (r'\bdropuser\s', 'dropuser'),

    # System/process
    (r'\bshutdown\s', 'shutdown'),
    (r'\breboot\b', 'reboot'),
    (r'\bhalt\b', 'halt'),
    (r'\bpoweroff\b', 'poweroff'),
    (r'launchctl\s+(unload|remove|bootout)', 'launchctl unload/remove'),
    (r'systemctl\s+(stop|disable)\s', 'systemctl stop/disable'),
    (r'crontab\s+-r\b', 'crontab -r'),
    (r'\bkillall\s', 'killall'),

    # Network exfil via pipe to shell (checked against FULL command before splitting)
    # These are handled separately below since pipe-splitting breaks the pattern

    # Expose local services to internet
    (r'python3?\s+-m\s+http\.server', 'python http.server'),
    (r'\bngrok\s', 'ngrok'),

    # macOS-specific dangers
    (r'\bosascript\s', 'osascript'),
    (r'\bdefaults\s+write\s', 'defaults write'),
    (r'\bdefaults\s+delete\s', 'defaults delete'),
    (r'\bdscl\s', 'dscl'),
    (r'\bscutil\s+--set\s', 'scutil --set'),
    (r'\bnetworksetup\s+-set', 'networksetup'),
    (r'\bcsrutil\s', 'csrutil'),
    (r'xattr\s+-c', 'xattr clear'),
    (r'\bpfctl\s', 'pfctl (firewall)'),

    # Credential/secret exfil
    (r'\bsecurity\s+find-(generic|internet)-password', 'keychain read'),
    (r'\bsecurity\s+delete-(generic|internet)-password', 'keychain delete'),
    (r'\bsecurity\s+dump-keychain', 'keychain dump'),
    (r'ssh-keygen\s+(?!.*-l).*-f\s', 'ssh-keygen overwrite'),  # allow -l -f (list)
    (r'gpg\s+--delete', 'gpg key delete'),

    # Persistence mechanisms
    (r'launchctl\s+(load|bootstrap)', 'launchctl load (persistence)'),
    (r'/Library/LaunchAgents', 'LaunchAgent modification'),
    (r'/Library/LaunchDaemons', 'LaunchDaemon modification'),

    # Clipboard exfiltration
    (r'\bpbcopy\b', 'pbcopy (clipboard write)'),

    # History access
    (r'\.zsh_history', 'zsh history access'),
    (r'\.bash_history', 'bash history access'),

    # Dangerous archive operations
    (r'tar\s+.*--overwrite', 'tar overwrite'),

    # Network/DNS changes
    (r'networksetup\s+-setdnsservers', 'DNS change'),
    (r'\biptables\s', 'iptables'),

    # Shell obfuscation / indirect execution
    (r'\beval\s+(?!\x22)', 'eval (not quoted subshell)'),
    (r'\{[a-z]+,', 'brace expansion (possible obfuscation)'),

    # Network tools (lateral movement / exfil)
    (r'\bssh\s', 'ssh'),
    (r'\bscp\s', 'scp'),
    (r'\brsync\s.*:', 'rsync to remote'),
    (r'\bnc\s', 'netcat'),
    (r'\bncat\s', 'ncat'),
    (r'\bsocat\s', 'socat'),

    # Mutating HTTP requests to APIs
    (r'curl\s.*-X\s*(DELETE|PUT|POST|PATCH)', 'curl mutating HTTP request'),
    (r'curl\s.*(-d\s|--data)', 'curl sending data'),
    (r'curl\s.*--upload-file', 'curl upload'),
    (r'curl\s.*-T\s', 'curl upload (-T)'),

    # Arbitrary code execution via scripting languages
    (r'\bnpx\s', 'npx (arbitrary code execution)'),

    # Symlink attacks
    (r'\bln\s+-[a-z]*s[a-z]*f', 'ln -sf (force symlink)'),

    # Crontab install (not just -r)
    (r'\bcrontab\s+\S+\.\S+', 'crontab install from file'),
]

# ── Pre-split checks (patterns that span pipes) ──
pipe_patterns = [
    (r'\|\s*(ba)?sh\b', 'pipe to shell'),  # generic: anything | sh or | bash
    (r'\|\s*zsh\b', 'pipe to zsh'),
]
for pattern, name in pipe_patterns:
    if re.search(pattern, command, re.IGNORECASE):
        result = {
            'hookSpecificOutput': {
                'permissionDecision': 'ask',
                'reason': f'Flagged [{name}]: {command[:80]}'
            }
        }
        print(json.dumps(result))
        sys.exit(0)

# ── Split compound command into subcommands ──
# Split on &&, ||, ;
parts = re.split(r'\s*(?:&&|\|\||;)\s*', command)

# Further split on pipe
expanded = []
for part in parts:
    expanded.extend(re.split(r'\s*\|\s*', part))

# Extract $() subshell contents
subshells = re.findall(r'\$\(([^)]+)\)', command)
expanded.extend(subshells)

# Clean each subcommand — keep both raw and cleaned versions
cleaned_parts = []
for sub in expanded:
    sub = sub.strip()
    if not sub:
        continue
    # Keep the raw version (with redirections) for checking
    raw = sub
    # Remove leading env vars: FOO=bar command -> command
    sub = re.sub(r'^(\w+=\S+\s+)*', '', sub)
    # Remove trailing redirections for the cleaned version
    cleaned = re.sub(r'\s*[12]?>[>&]?\s*\S+\s*$', '', sub)
    if sub.strip():
        # Add both raw (with redirects) and cleaned versions
        cleaned_parts.append(sub.strip())
        if cleaned.strip() and cleaned.strip() != sub.strip():
            cleaned_parts.append(cleaned.strip())

# Case-sensitive patterns (e.g. git branch -D vs -d)
CASE_SENSITIVE_PATTERNS = [
    (r'git\s+branch\s+-D\s', 'git branch -D'),
]

# ── Check each subcommand against deny patterns ──
for sub in cleaned_parts:
    for pattern, name in CASE_SENSITIVE_PATTERNS:
        if re.search(pattern, sub):
            result = {
                'hookSpecificOutput': {
                    'permissionDecision': 'ask',
                    'reason': f'Flagged [{name}]: {sub}'
                }
            }
            print(json.dumps(result))
            sys.exit(0)
    for pattern, name in DENY_PATTERNS:
        if re.search(pattern, sub, re.IGNORECASE):
            result = {
                'hookSpecificOutput': {
                    'permissionDecision': 'ask',
                    'reason': f'Flagged [{name}]: {sub}'
                }
            }
            print(json.dumps(result))
            sys.exit(0)

# No deny patterns matched — auto-approve
sys.exit(0)
" <<< "$INPUT")

if [ -n "$RESULT" ]; then
    echo "$RESULT"
fi

exit 0