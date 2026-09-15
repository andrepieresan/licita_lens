#!/usr/bin/env bash
set -euo pipefail
IP="$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || true)"
if [[ -z "$IP" ]]; then
  echo "Não foi possível detectar o IP da rede local. Defina EXPO_PUBLIC_API_URL manualmente." >&2
  exit 1
fi
export EXPO_PUBLIC_API_URL="http://${IP}:8080"
echo "→ API para o celular: ${EXPO_PUBLIC_API_URL}"
echo "→ Expo Go (URL manual): exp://${IP}:8081"
echo ""
echo "  Se abrir app antigo: feche outros 'expo start' no Mac (portas 8081/8082)"
echo "  No Expo Go → Projects → segure o projeto antigo → Remove"
echo "  Depois conecte de novo só com exp://${IP}:8081"
echo ""
exec expo start --clear
