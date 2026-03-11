#!/usr/bin/env bash

RUNTIME_RECIPE_VERSION=3
RUNTIME_NAMES=(node22 node24 python3.13)
RUNTIME_ARCHES=(linux-amd64 linux-arm64)

runtime_list() {
  printf '%s\n' "${RUNTIME_NAMES[@]}"
}

runtime_arch_list() {
  printf '%s\n' "${RUNTIME_ARCHES[@]}"
}

runtime_base_image() {
  case "${1:-}" in
    node22)
      printf 'node:22\n'
      ;;
    node24)
      printf 'node:24\n'
      ;;
    python3.13)
      printf 'python:3.13-slim\n'
      ;;
    *)
      return 1
      ;;
  esac
}

runtime_filename() {
  case "${1:-}" in
    node22)
      printf 'node22.ext4\n'
      ;;
    node24)
      printf 'node24.ext4\n'
      ;;
    python3.13)
      printf 'python3.13.ext4\n'
      ;;
    *)
      return 1
      ;;
  esac
}

runtime_arch_dir() {
  case "${1:-}" in
    linux-amd64 | linux/amd64 | amd64 | x86_64)
      printf 'linux-amd64\n'
      ;;
    linux-arm64 | linux/arm64 | arm64 | aarch64)
      printf 'linux-arm64\n'
      ;;
    *)
      return 1
      ;;
  esac
}

runtime_docker_platform() {
  case "$(runtime_arch_dir "${1:-}")" in
    linux-amd64)
      printf 'linux/amd64\n'
      ;;
    linux-arm64)
      printf 'linux/arm64\n'
      ;;
    *)
      return 1
      ;;
  esac
}
