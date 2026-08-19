#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DIST_DIR="${SCRIPT_DIR}/dist"

TARGET_MATRIX=(
  "windows-amd64|windows|amd64||go2rtc_win64.exe"
  "windows-386|windows|386||go2rtc_win32.exe"
  "windows-arm64|windows|arm64||go2rtc_win_arm64.exe"
  "linux-amd64|linux|amd64||go2rtc_linux_amd64"
  "linux-386|linux|386||go2rtc_linux_i386"
  "linux-arm64|linux|arm64||go2rtc_linux_arm64"
  "linux-armv7|linux|arm|7|go2rtc_linux_arm"
  "linux-armv6|linux|arm|6|go2rtc_linux_armv6"
  "linux-mipsle|linux|mipsle||go2rtc_linux_mipsel"
  "darwin-amd64|darwin|amd64||go2rtc_mac_amd64"
  "darwin-arm64|darwin|arm64||go2rtc_mac_arm64"
  "freebsd-amd64|freebsd|amd64||go2rtc_freebsd_amd64"
  "freebsd-arm64|freebsd|arm64||go2rtc_freebsd_arm64"
)

show_help() {
  cat <<'EOF'
用法：
  ./packaging/build.sh [选项] [目标...]

目标：
  all、windows、linux、darwin（mac、macos）、freebsd
  或具体目标，例如 linux-amd64、linux-arm64、windows-amd64

选项：
  --clean       构建前清空 packaging/dist
  --skip-test   跳过 go test -count=1 ./internal/api
  --list        显示所有目标
  -h, --help    显示帮助
EOF
}

show_targets() {
  printf '%-18s %-9s %-7s %-5s %s\n' 'TARGET' 'GOOS' 'GOARCH' 'ARM' 'OUTPUT'
  local record name goos goarch goarm output
  for record in "${TARGET_MATRIX[@]}"; do
    IFS='|' read -r name goos goarch goarm output <<<"${record}"
    printf '%-18s %-9s %-7s %-5s %s\n' "${name}" "${goos}" "${goarch}" "${goarm:--}" "${output}"
  done
}

record_name() {
  printf '%s' "${1%%|*}"
}

add_record() {
  local candidate="$1" existing
  for existing in "${SELECTED_RECORDS[@]:-}"; do
    if [[ "$(record_name "${existing}")" == "$(record_name "${candidate}")" ]]; then
      return
    fi
  done
  SELECTED_RECORDS+=("${candidate}")
}

select_target() {
  local selector="$1" record name goos goarch goarm output matched=0 found=0
  selector="$(printf '%s' "${selector}" | tr '[:upper:]' '[:lower:]')"
  for record in "${TARGET_MATRIX[@]}"; do
    IFS='|' read -r name goos goarch goarm output <<<"${record}"
    case "${selector}" in
      all) matched=1 ;;
      windows|linux|freebsd) [[ "${goos}" == "${selector}" ]] && matched=1 || matched=0 ;;
      darwin|mac|macos) [[ "${goos}" == "darwin" ]] && matched=1 || matched=0 ;;
      *) [[ "${name}" == "${selector}" ]] && matched=1 || matched=0 ;;
    esac
    if [[ ${matched} -eq 1 ]]; then
      add_record "${record}"
      found=1
    fi
  done
  if [[ ${found} -eq 0 ]]; then
    printf '未知构建目标：%s。使用 --list 查看可用目标。\n' "${selector}" >&2
    exit 1
  fi
}

CLEAN=0
SKIP_TEST=0
LIST_ONLY=0
SELECTORS=()
SELECTED_RECORDS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --clean) CLEAN=1 ;;
    --skip-test) SKIP_TEST=1 ;;
    --list) LIST_ONLY=1 ;;
    -h|--help) show_help; exit 0 ;;
    --*) printf '未知选项：%s\n' "$1" >&2; show_help; exit 1 ;;
    *) SELECTORS+=("$1") ;;
  esac
  shift
done

if [[ ${LIST_ONLY} -eq 1 ]]; then
  show_targets
  exit 0
fi

if ! command -v go >/dev/null 2>&1; then
  printf '未找到 Go，请先安装 Go 1.25 或更高版本并加入 PATH。\n' >&2
  exit 1
fi

if [[ ${#SELECTORS[@]} -eq 0 ]]; then
  SELECTORS=('all')
fi

for raw_selector in "${SELECTORS[@]}"; do
  IFS=',' read -r -a split_selectors <<<"${raw_selector}"
  for selector in "${split_selectors[@]}"; do
    select_target "${selector}"
  done
done

mkdir -p "${DIST_DIR}"
if [[ ${CLEAN} -eq 1 ]]; then
  if [[ "${DIST_DIR}" != "${SCRIPT_DIR}/dist" ]]; then
    printf '拒绝清理非预期目录：%s\n' "${DIST_DIR}" >&2
    exit 1
  fi
  rm -rf -- "${DIST_DIR}"
  mkdir -p "${DIST_DIR}"
fi

if [[ ${SKIP_TEST} -eq 0 ]]; then
  printf '==> 运行构建前测试：go test -count=1 ./internal/api\n'
  (cd "${REPO_ROOT}" && go test -count=1 ./internal/api)
fi

index=0
total=${#SELECTED_RECORDS[@]}
for record in "${SELECTED_RECORDS[@]}"; do
  index=$((index + 1))
  IFS='|' read -r name goos goarch goarm output <<<"${record}"
  printf '==> [%d/%d] %s -> %s\n' "${index}" "${total}" "${name}" "${output}"

  env_args=("CGO_ENABLED=0" "GOOS=${goos}" "GOARCH=${goarch}")
  if [[ -n "${goarm}" ]]; then
    env_args+=("GOARM=${goarm}")
  fi
  (cd "${REPO_ROOT}" && env "${env_args[@]}" go build -ldflags '-s -w' -trimpath -o "${DIST_DIR}/${output}" .)
done

checksum_file="${DIST_DIR}/SHA256SUMS.txt"
: >"${checksum_file}"
while IFS= read -r file_path; do
  output="$(basename "${file_path}")"
  if command -v sha256sum >/dev/null 2>&1; then
    hash="$(sha256sum "${file_path}" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    hash="$(shasum -a 256 "${file_path}" | awk '{print $1}')"
  else
    printf '未找到 sha256sum 或 shasum，无法生成校验清单。\n' >&2
    exit 1
  fi
  printf '%s  %s\n' "${hash}" "${output}" >>"${checksum_file}"
done < <(find "${DIST_DIR}" -maxdepth 1 -type f ! -name 'SHA256SUMS.txt' ! -name '.gitignore' | sort)

printf '\n构建完成：%s\n' "${DIST_DIR}"
printf 'SHA256 清单：%s\n' "${checksum_file}"
