# AIR API Design

[中文](api-design.md)

## 1. Design Principles

- predictable interfaces
- structured responses
- clear error handling
- stable fields for agent workflows

## 2. Common Conventions

- base URL example: `http://127.0.0.1:8080`
- JSON request and response payloads
- shared response fields should include `success`, `error_type`, and `error_message`

## 3. Run API

A one-shot endpoint should accept a command plus runtime options and return `stdout`, `stderr`, `exit_code`, timeout state, duration, and request identifiers.

## 4. Session APIs

- create session
- inspect session
- exec inside session
- delete session

These endpoints should expose the minimum metadata an agent needs to continue a workflow safely.

## 5. Health Endpoint

A simple health check endpoint is useful for control-plane readiness and integration checks.

## 6. Error Codes

Errors should distinguish runtime startup failures, transport failures, guest execution failures, timeouts, and invalid requests.

## 7. Future Extensions

Streaming output, richer lifecycle controls, and stronger auth can be added later without breaking the basic structured contract.
