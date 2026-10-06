#!/usr/bin/env bash
set -euo pipefail

# Проверяет staged-файлы на UTF-8 BOM и завершает commit с ошибкой.

mapfile -d '' files < <(git diff --cached --name-only -z --diff-filter=ACM)

if [[ ${#files[@]} -eq 0 ]]; then
  exit 0
fi

bad=()
for f in "${files[@]}"; do
  [[ -f "$f" ]] || continue
  case "$f" in
    *.png|*.jpg|*.jpeg|*.gif|*.webp|*.ico|*.pdf|*.woff|*.woff2|*.ttf|*.eot|*.gz|*.zip)
      continue
      ;;
  esac

  first3="$(LC_ALL=C head -c 3 "$f" | od -An -t x1 | tr -d ' \n')"
  if [[ "$first3" == "efbbbf" ]]; then
    bad+=("$f")
  fi
done

if [[ ${#bad[@]} -gt 0 ]]; then
  echo "ERROR: UTF-8 BOM обнаружен в файлах:"
  for f in "${bad[@]}"; do
    echo "  - $f"
  done
  echo
  echo "Исправить: sed -i '1s/^\\xEF\\xBB\\xBF//' <file>"
  exit 1
fi
