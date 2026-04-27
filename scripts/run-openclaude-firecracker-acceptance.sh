#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_DIR="${ARTIFACT_DIR:-artifacts/openclaude-firecracker-acceptance}"
OPENCLAUDE_REPO="${OPENCLAUDE_REPO:-$HOME/Documents/code/openclaude}"
WORKSPACE="${WORKSPACE:-}"
TASK_MESSAGE="${TASK_MESSAGE:-Create a file named AIR_OPENCLAUDE_ACCEPTANCE.txt containing exactly AIR_OPENCLAUDE_ACCEPTANCE_OK, then finish.}"
EXPORT_DIR="${EXPORT_DIR:-${ARTIFACT_DIR}/workspace-export}"
FORWARD_ADDR="${FORWARD_ADDR:-127.0.0.1:50052}"
KEEP_SESSION_ON_FAILURE="${KEEP_SESSION_ON_FAILURE:-0}"

mkdir -p "${ARTIFACT_DIR}"
LOG_FILE="${ARTIFACT_DIR}/acceptance.log"
RESULT_FILE="${ARTIFACT_DIR}/result.json"
: >"${LOG_FILE}"

log() {
  printf '%s\n' "$*" | tee -a "${LOG_FILE}"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log "missing required command: $1"
    exit 1
  fi
}

load_key_file() {
  local env_name="$1"
  local file_name="${env_name}_FILE"
  local file_path="${!file_name:-}"
  if [[ -n "${file_path}" && -z "${!env_name:-}" ]]; then
    if [[ ! -r "${file_path}" ]]; then
      log "${file_name} is not readable: ${file_path}"
      exit 1
    fi
    export "${env_name}=$(tr -d '\r\n' <"${file_path}")"
  fi
}

require_cmd go
require_cmd bun

load_key_file OPENAI_API_KEY
load_key_file DEEPSEEK_API_KEY

if [[ ! -d "${OPENCLAUDE_REPO}" ]]; then
  log "OPENCLAUDE_REPO not found: ${OPENCLAUDE_REPO}"
  exit 1
fi
if [[ ! -f "${OPENCLAUDE_REPO}/scripts/start-grpc.ts" ]]; then
  log "OPENCLAUDE_REPO missing scripts/start-grpc.ts: ${OPENCLAUDE_REPO}"
  exit 1
fi

if [[ -z "${WORKSPACE}" ]]; then
  WORKSPACE="$(mktemp -d)"
  printf 'AIR OpenClaude Firecracker acceptance workspace\n' >"${WORKSPACE}/README.md"
fi
WORKSPACE="$(cd "${WORKSPACE}" && pwd)"

export CLAUDE_CODE_USE_OPENAI="${CLAUDE_CODE_USE_OPENAI:-1}"
if [[ -n "${DEEPSEEK_API_KEY:-}" && -z "${OPENAI_API_KEY:-}" ]]; then
  export OPENAI_API_KEY="${DEEPSEEK_API_KEY}"
  export OPENAI_BASE_URL="${OPENAI_BASE_URL:-${DEEPSEEK_BASE_URL:-https://api.deepseek.com/v1}}"
  export OPENAI_MODEL="${OPENAI_MODEL:-${DEEPSEEK_MODEL:-deepseek-chat}}"
else
  export OPENAI_BASE_URL="${OPENAI_BASE_URL:-https://api.openai.com/v1}"
  export OPENAI_MODEL="${OPENAI_MODEL:-gpt-4o-mini}"
fi

if [[ -z "${OPENAI_API_KEY:-}" ]]; then
  log "OPENAI_API_KEY or DEEPSEEK_API_KEY is required"
  exit 1
fi

log "OpenClaude Firecracker acceptance"
log "openclaude_repo=${OPENCLAUDE_REPO}"
log "workspace=${WORKSPACE}"
log "provider_base_url=${OPENAI_BASE_URL}"
log "provider_model=${OPENAI_MODEL}"
log "artifact_dir=${ARTIFACT_DIR}"

session_id=""
forward_pid=""
cleanup() {
  if [[ -n "${forward_pid}" ]]; then
    kill "${forward_pid}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${session_id}" && "${KEEP_SESSION_ON_FAILURE}" != "1" ]]; then
    go run ./cmd/air agent openclaude stop "${session_id}" >>"${LOG_FILE}" 2>&1 || true
    go run ./cmd/air session delete "${session_id}" >>"${LOG_FILE}" 2>&1 || true
  fi
}
trap cleanup EXIT

start_json="${ARTIFACT_DIR}/openclaude-start.json"
go run ./cmd/air agent openclaude start \
  --provider firecracker \
  --guest-repo /opt/openclaude \
  --workspace "${WORKSPACE}" >"${start_json}" 2>>"${LOG_FILE}"

session_id="$(sed -n 's/.*"session_id": *"\([^"]*\)".*/\1/p' "${start_json}" | head -n1)"
if [[ -z "${session_id}" ]]; then
  log "failed to parse session_id from ${start_json}"
  cat "${start_json}" >>"${LOG_FILE}"
  exit 1
fi
log "session_id=${session_id}"

go run ./cmd/air agent openclaude forward "${session_id}" --listen "${FORWARD_ADDR}" >>"${LOG_FILE}" 2>&1 &
forward_pid="$!"

for _ in $(seq 1 100); do
  if ! kill -0 "${forward_pid}" 2>/dev/null; then
    log "openclaude forward process exited early"
    exit 1
  fi
  if go run ./cmd/air agent openclaude status "${session_id}" >/dev/null 2>>"${LOG_FILE}"; then
    break
  fi
  sleep 0.2
done

client_ts="${OPENCLAUDE_REPO}/.air/openclaude-acceptance-client.ts"
mkdir -p "$(dirname "${client_ts}")"
cat >"${client_ts}" <<'CLIENT'
import * as grpc from '@grpc/grpc-js'
import * as protoLoader from '@grpc/proto-loader'
import path from 'path'

const repo = process.env.OPENCLAUDE_REPO!
const protoPath = path.join(repo, 'src/proto/openclaude.proto')
const packageDefinition = protoLoader.loadSync(protoPath, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true,
})
const protoDescriptor = grpc.loadPackageDefinition(packageDefinition) as any
const client = new protoDescriptor.openclaude.v1.AgentService(
  process.env.FORWARD_ADDR || '127.0.0.1:50052',
  grpc.credentials.createInsecure()
)

const call = client.Chat()
const transcript: any[] = []
const timeout = setTimeout(() => {
  console.error('timeout waiting for OpenClaude response')
  call.destroy()
  process.exit(124)
}, Number(process.env.ACCEPTANCE_TIMEOUT_MS || 180000))

call.on('data', (message: any) => {
  transcript.push(message)
  process.stdout.write(JSON.stringify(message) + '\n')
  if (message.action_required) {
    call.write({ input: { prompt_id: message.action_required.prompt_id, reply: 'y' } })
  }
  if (message.done) {
    clearTimeout(timeout)
    call.end()
    process.exit(0)
  }
  if (message.error) {
    clearTimeout(timeout)
    call.end()
    process.exit(2)
  }
})
call.on('error', err => {
  clearTimeout(timeout)
  console.error(err.message)
  process.exit(1)
})
call.write({
  request: {
    session_id: process.env.OPENCLAUDE_ACCEPTANCE_SESSION || 'air-openclaude-firecracker-acceptance',
    message: process.env.TASK_MESSAGE,
    working_directory: '/workspace',
  },
})
CLIENT

OPENCLAUDE_REPO="${OPENCLAUDE_REPO}" \
FORWARD_ADDR="${FORWARD_ADDR}" \
TASK_MESSAGE="${TASK_MESSAGE}" \
bun "${client_ts}" >"${ARTIFACT_DIR}/grpc-transcript.jsonl" 2>>"${LOG_FILE}"
rm -f "${client_ts}"

go run ./cmd/air session export-workspace "${session_id}" "${EXPORT_DIR}" --force >"${ARTIFACT_DIR}/workspace-export.json" 2>>"${LOG_FILE}"

if [[ ! -f "${EXPORT_DIR}/AIR_OPENCLAUDE_ACCEPTANCE.txt" ]]; then
  log "acceptance output file missing"
  exit 1
fi
if [[ "$(tr -d '\r\n' <"${EXPORT_DIR}/AIR_OPENCLAUDE_ACCEPTANCE.txt")" != "AIR_OPENCLAUDE_ACCEPTANCE_OK" ]]; then
  log "acceptance output file content mismatch"
  exit 1
fi

cat >"${RESULT_FILE}" <<EOF
{
  "success": true,
  "session_id": "${session_id}",
  "workspace": "${WORKSPACE}",
  "export_dir": "${EXPORT_DIR}",
  "provider_base_url": "${OPENAI_BASE_URL}",
  "provider_model": "${OPENAI_MODEL}"
}
EOF
log "success=true"
