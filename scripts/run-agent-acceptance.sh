#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/run-agent-acceptance.sh [options]

Options:
  --planner <openai|deepseek|scripted>   Planner backend. Default: scripted
  --provider <local|firecracker>         AIR runtime provider. Default: local
  --task <name>                          Task name. Default: all
  --model <name>                         Planner model override
  --escalation-model <name>              Planner escalation model override
  --planner-retries <count>              Planner retries before escalation
  --reasoning <level>                    Planner reasoning override
  --output-dir <path>                    Output directory for JSON artifacts
  -h, --help                             Show this help

Environment:
  GOCACHE / GOMODCACHE                   Optional Go cache overrides
  OPENAI_API_KEY / OPENAI_API_KEY_FILE   OpenAI API key or key file path
  DEEPSEEK_API_KEY / DEEPSEEK_API_KEY_FILE
                                        DeepSeek API key or key file path

Examples:
  scripts/run-agent-acceptance.sh --planner scripted --task all
  scripts/run-agent-acceptance.sh --planner deepseek --model deepseek-chat \
    --escalation-model deepseek-reasoner --planner-retries 1 --task all
  OPENAI_API_KEY_FILE=~/tmp/openai.api scripts/run-agent-acceptance.sh \
    --planner openai --model gpt-5.4-mini --escalation-model gpt-5.4
EOF
}

require_key() {
  local env_name="$1"
  local file_env_name="$2"
  local current_value="${!env_name:-}"
  local file_path="${!file_env_name:-}"

  if [[ -n "${current_value}" ]]; then
    return 0
  fi

  if [[ -n "${file_path}" ]]; then
    if [[ ! -r "${file_path}" ]]; then
      echo "cannot read ${file_env_name}: ${file_path}" >&2
      exit 1
    fi
    export "${env_name}=$(tr -d '\r\n' < "${file_path}")"
    return 0
  fi

  echo "missing ${env_name}; set ${env_name} or ${file_env_name}" >&2
  exit 1
}

planner="scripted"
provider="local"
task="all"
model=""
escalation_model=""
planner_retries=""
reasoning=""
output_dir=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --planner)
      planner="$2"
      shift 2
      ;;
    --provider)
      provider="$2"
      shift 2
      ;;
    --task)
      task="$2"
      shift 2
      ;;
    --model)
      model="$2"
      shift 2
      ;;
    --escalation-model)
      escalation_model="$2"
      shift 2
      ;;
    --planner-retries)
      planner_retries="$2"
      shift 2
      ;;
    --reasoning)
      reasoning="$2"
      shift 2
      ;;
    --output-dir)
      output_dir="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"

if [[ -z "${model}" ]]; then
  model="${AIR_AGENT_MODEL:-}"
fi
if [[ -z "${escalation_model}" ]]; then
  escalation_model="${AIR_AGENT_ESCALATION_MODEL:-}"
fi
if [[ -z "${planner_retries}" ]]; then
  planner_retries="${AIR_AGENT_PLANNER_RETRIES:-1}"
fi
if [[ -z "${reasoning}" ]]; then
  reasoning="${AIR_AGENT_REASONING:-}"
fi

: "${GOCACHE:=/tmp/go-build}"
: "${GOMODCACHE:=/tmp/gomodcache}"
export GOCACHE
export GOMODCACHE

if [[ -z "${output_dir}" ]]; then
  output_dir="${ROOT_DIR}/artifacts/agent-acceptance/${planner}-${provider}-${task}-${STAMP}"
fi
mkdir -p "${output_dir}"

case "${planner}" in
  scripted)
    ;;
  openai)
    require_key "OPENAI_API_KEY" "OPENAI_API_KEY_FILE"
    ;;
  deepseek)
    require_key "DEEPSEEK_API_KEY" "DEEPSEEK_API_KEY_FILE"
    ;;
  *)
    echo "unsupported planner: ${planner}" >&2
    exit 1
    ;;
esac

declare -a cmd=(
  go run ./examples/agent-runner
  --planner "${planner}"
  --provider "${provider}"
  --task "${task}"
)

if [[ -n "${model}" ]]; then
  cmd+=(--model "${model}")
fi
if [[ -n "${escalation_model}" ]]; then
  cmd+=(--escalation-model "${escalation_model}")
fi
if [[ -n "${planner_retries}" ]]; then
  cmd+=(--planner-retries "${planner_retries}")
fi
if [[ -n "${reasoning}" ]]; then
  cmd+=(--reasoning "${reasoning}")
fi

command_file="${output_dir}/command.txt"
result_file="${output_dir}/result.json"
meta_file="${output_dir}/metadata.txt"

printf '%q ' "${cmd[@]}" > "${command_file}"
printf '\n' >> "${command_file}"

cat > "${meta_file}" <<EOF
planner=${planner}
provider=${provider}
task=${task}
model=${model}
escalation_model=${escalation_model}
planner_retries=${planner_retries}
reasoning=${reasoning}
started_at=${STAMP}
cwd=${ROOT_DIR}
EOF

(
  cd "${ROOT_DIR}"
  "${cmd[@]}"
) > "${result_file}"

echo "Acceptance run completed."
echo "  command:  ${command_file}"
echo "  metadata: ${meta_file}"
echo "  result:   ${result_file}"
