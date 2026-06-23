#!/bin/bash
# Soft-Proxy Protocol Test — Extended with SNI Random
set -e

DOMAIN="test2.tunnel.sryze.cc"
RANDOM_SNI="random.test.example"
SOCKS_PORT=10808
XRAY_BIN="/usr/local/bin/xray"
PINNED="2be33aa800df81cb57ee29dc90251ed61aa1198356729ea855fe97fbbfc09dc6"

PASS=0; FAIL=0

run_client() {
    local client_json="$1" test_name="$2"
    local tmpfile=$(mktemp /tmp/xray-test-XXXXXX.json)
    echo "$client_json" > "$tmpfile"
    $XRAY_BIN run -config "$tmpfile" > /tmp/xray-client.log 2>&1 &
    local pid=$!
    sleep 2
    if ! kill -0 "$pid" 2>/dev/null; then
        echo "  FAIL: $test_name (xray client failed to start)"
        rm -f "$tmpfile"; FAIL=$((FAIL+1)); return 1
    fi
    local result=$(curl -s --max-time 10 -x socks5://127.0.0.1:$SOCKS_PORT https://httpbin.org/ip 2>&1 || true)
    kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; sleep 1
    rm -f "$tmpfile"
    if echo "$result" | grep -q '"origin"'; then
        echo "  PASS: $test_name"; PASS=$((PASS+1)); return 0
    else
        echo "  FAIL: $test_name (${result:0:80})"
        FAIL=$((FAIL+1)); return 1
    fi
}

VLESS_ID="09abf07d-a8ea-4748-9f37-5b3dca0e0a94"
VLESS_RID="e75a1d12-7c68-4971-b1fb-3f7fe767c6d6"
VMESS_ID="8e42b478-3a4b-48b0-ad2e-a58625acda8e"
TROJAN_PASS="140141d26c7dfa2171cf1cc460190ba2"
REALITY_PBK="Y07pOrSNdp7YtiCXffp64UoTanx1J4LK_YX8HkHs_is"
REALITY_SID="01234567"
TLS="\"tlsSettings\":{\"serverName\":\"$DOMAIN\"}"
TLS_RANDOM="\"tlsSettings\":{\"serverName\":\"$RANDOM_SNI\",\"pinnedPeerCertSha256\":\"$PINNED\"}"
HOST="\"host\":\"$DOMAIN\""

# ===================== PORT 443 TLS — DOMAIN =====================
echo "===== PORT 443 TLS (Domain: $DOMAIN) ====="

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"tcp","security":"tls",'"$TLS"'}}]}' "VLESS TCP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"ws","security":"tls",'"$TLS"',"wsSettings":{"path":"/vless-ws",'"$HOST"'}}}]}' "VLESS WS"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"httpupgrade","security":"tls",'"$TLS"',"httpupgradeSettings":{"path":"/vless-httpupgrade",'"$HOST"'}}}]}' "VLESS HTTPUpgrade"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"grpc","security":"tls",'"$TLS"',"grpcSettings":{"serviceName":"vless-grpc"}}}]}' "VLESS gRPC"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"xhttp","security":"tls",'"$TLS"',"xhttpSettings":{"path":"/vless-xhttp"}}}]}' "VLESS XHTTP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"tcp","security":"tls",'"$TLS"'}}]}' "VMess TCP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"ws","security":"tls",'"$TLS"',"wsSettings":{"path":"/vmess-ws",'"$HOST"'}}}]}' "VMess WS"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"httpupgrade","security":"tls",'"$TLS"',"httpupgradeSettings":{"path":"/vmess-httpupgrade",'"$HOST"'}}}]}' "VMess HTTPUpgrade"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"grpc","security":"tls",'"$TLS"',"grpcSettings":{"serviceName":"vmess-grpc"}}}]}' "VMess gRPC"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"xhttp","security":"tls",'"$TLS"',"xhttpSettings":{"path":"/vmess-xhttp"}}}]}' "VMess XHTTP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":443,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"tcp","security":"tls",'"$TLS"'}}]}' "Trojan TCP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":443,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"ws","security":"tls",'"$TLS"',"wsSettings":{"path":"/trojan-ws",'"$HOST"'}}}]}' "Trojan WS"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":443,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"httpupgrade","security":"tls",'"$TLS"',"httpupgradeSettings":{"path":"/trojan-httpupgrade",'"$HOST"'}}}]}' "Trojan HTTPUpgrade"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":443,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"grpc","security":"tls",'"$TLS"',"grpcSettings":{"serviceName":"trojan-grpc"}}}]}' "Trojan gRPC"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":443,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"xhttp","security":"tls",'"$TLS"',"xhttpSettings":{"path":"/trojan-xhttp"}}}]}' "Trojan XHTTP"

# ===================== PORT 443 TLS — RANDOM SNI =====================
echo ""
echo "===== PORT 443 TLS (SNI: $RANDOM_SNI — self-signed fallback) ====="

R_HOST="\"host\":\"$RANDOM_SNI\""

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"tcp","security":"tls",'"$TLS_RANDOM"'}}]}' "VLESS TCP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"ws","security":"tls",'"$TLS_RANDOM"',"wsSettings":{"path":"/vless-ws",'"$R_HOST"'}}}]}' "VLESS WS"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"httpupgrade","security":"tls",'"$TLS_RANDOM"',"httpupgradeSettings":{"path":"/vless-httpupgrade",'"$R_HOST"'}}}]}' "VLESS HTTPUpgrade"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"grpc","security":"tls",'"$TLS_RANDOM"',"grpcSettings":{"serviceName":"vless-grpc"}}}]}' "VLESS gRPC"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"xhttp","security":"tls",'"$TLS_RANDOM"',"xhttpSettings":{"path":"/vless-xhttp"}}}]}' "VLESS XHTTP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"tcp","security":"tls",'"$TLS_RANDOM"'}}]}' "VMess TCP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"ws","security":"tls",'"$TLS_RANDOM"',"wsSettings":{"path":"/vmess-ws",'"$R_HOST"'}}}]}' "VMess WS"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"httpupgrade","security":"tls",'"$TLS_RANDOM"',"httpupgradeSettings":{"path":"/vmess-httpupgrade",'"$R_HOST"'}}}]}' "VMess HTTPUpgrade"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"grpc","security":"tls",'"$TLS_RANDOM"',"grpcSettings":{"serviceName":"vmess-grpc"}}}]}' "VMess gRPC"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"xhttp","security":"tls",'"$TLS_RANDOM"',"xhttpSettings":{"path":"/vmess-xhttp"}}}]}' "VMess XHTTP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":443,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"tcp","security":"tls",'"$TLS_RANDOM"'}}]}' "Trojan TCP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":443,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"ws","security":"tls",'"$TLS_RANDOM"',"wsSettings":{"path":"/trojan-ws",'"$R_HOST"'}}}]}' "Trojan WS"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":443,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"httpupgrade","security":"tls",'"$TLS_RANDOM"',"httpupgradeSettings":{"path":"/trojan-httpupgrade",'"$R_HOST"'}}}]}' "Trojan HTTPUpgrade"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":443,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"grpc","security":"tls",'"$TLS_RANDOM"',"grpcSettings":{"serviceName":"trojan-grpc"}}}]}' "Trojan gRPC"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":443,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"xhttp","security":"tls",'"$TLS_RANDOM"',"xhttpSettings":{"path":"/trojan-xhttp"}}}]}' "Trojan XHTTP"

# ===================== PORT 443 REALITY =====================
echo ""
echo "===== PORT 443 REALITY ====="

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_RID'","encryption":"none","flow":"xtls-rprx-vision"}]}]},"streamSettings":{"network":"tcp","security":"reality","realitySettings":{"fingerprint":"chrome","serverName":"yahoo.com","publicKey":"'$REALITY_PBK'","shortId":"'$REALITY_SID'","spiderX":"/"}}}]}' "VLESS Reality Vision"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"xhttp","security":"reality","realitySettings":{"fingerprint":"chrome","serverName":"www.google.com","publicKey":"'$REALITY_PBK'","shortId":"'$REALITY_SID'"},"xhttpSettings":{"path":"/vless-xhttp-reality"}}}]}' "VLESS Reality XHTTP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"tcp","security":"reality","realitySettings":{"fingerprint":"chrome","serverName":"www.yahoo.com","publicKey":"'$REALITY_PBK'","shortId":"'$REALITY_SID'"}}}]}' "VLESS Reality TCP NoFlow"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"tcp","security":"reality","realitySettings":{"fingerprint":"chrome","serverName":"www.cisco.com","publicKey":"'$REALITY_PBK'","shortId":"'$REALITY_SID'"}}}]}' "VMess Reality TCP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"xhttp","security":"reality","realitySettings":{"fingerprint":"chrome","serverName":"www.speedtest.net","publicKey":"'$REALITY_PBK'","shortId":"'$REALITY_SID'"},"xhttpSettings":{"path":"/vmess-xhttp-reality"}}}]}' "VMess Reality XHTTP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":443,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"tcp","security":"reality","realitySettings":{"fingerprint":"chrome","serverName":"www.bing.com","publicKey":"'$REALITY_PBK'","shortId":"'$REALITY_SID'"}}}]}' "VMess Reality TCP NoFlow"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":443,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"tcp","security":"reality","realitySettings":{"fingerprint":"chrome","serverName":"apple.com","publicKey":"'$REALITY_PBK'","shortId":"'$REALITY_SID'"}}}]}' "Trojan Reality TCP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":443,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"xhttp","security":"reality","realitySettings":{"fingerprint":"chrome","serverName":"www.icloud.com","publicKey":"'$REALITY_PBK'","shortId":"'$REALITY_SID'"},"xhttpSettings":{"path":"/trojan-xhttp-reality"}}}]}' "Trojan Reality XHTTP"

# ===================== PORT 80 PLAIN =====================
echo ""
echo "===== PORT 80 PLAIN ====="

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":80,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"tcp","security":"none"}}]}' "VLESS TCP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":80,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"ws","security":"none","wsSettings":{"path":"/vless-ws",'"$HOST"'}}}]}' "VLESS WS"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":80,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"httpupgrade","security":"none","httpupgradeSettings":{"path":"/vless-httpupgrade",'"$HOST"'}}}]}' "VLESS HTTPUpgrade"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":80,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"grpc","security":"none","grpcSettings":{"serviceName":"vless-grpc"}}}]}' "VLESS gRPC"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"'$DOMAIN'","port":80,"users":[{"id":"'$VLESS_ID'","encryption":"none"}]}]},"streamSettings":{"network":"xhttp","security":"none","xhttpSettings":{"path":"/vless-xhttp"}}}]}' "VLESS XHTTP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":80,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"tcp","security":"none"}}]}' "VMess TCP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":80,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"ws","security":"none","wsSettings":{"path":"/vmess-ws",'"$HOST"'}}}]}' "VMess WS"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":80,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"httpupgrade","security":"none","httpupgradeSettings":{"path":"/vmess-httpupgrade",'"$HOST"'}}}]}' "VMess HTTPUpgrade"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":80,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"grpc","security":"none","grpcSettings":{"serviceName":"vmess-grpc"}}}]}' "VMess gRPC"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"vmess","settings":{"vnext":[{"address":"'$DOMAIN'","port":80,"users":[{"id":"'$VMESS_ID'","alterId":0}]}]},"streamSettings":{"network":"xhttp","security":"none","xhttpSettings":{"path":"/vmess-xhttp"}}}]}' "VMess XHTTP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":80,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"tcp","security":"none"}}]}' "Trojan TCP"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":80,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"ws","security":"none","wsSettings":{"path":"/trojan-ws",'"$HOST"'}}}]}' "Trojan WS"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":80,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"httpupgrade","security":"none","httpupgradeSettings":{"path":"/trojan-httpupgrade",'"$HOST"'}}}]}' "Trojan HTTPUpgrade"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":80,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"grpc","security":"none","grpcSettings":{"serviceName":"trojan-grpc"}}}]}' "Trojan gRPC"

run_client '{"log":{"loglevel":"warning"},"inbounds":[{"port":10808,"listen":"127.0.0.1","protocol":"socks","settings":{"auth":"noauth","udp":true}}],"outbounds":[{"protocol":"trojan","settings":{"servers":[{"address":"'$DOMAIN'","port":80,"password":"'$TROJAN_PASS'"}]},"streamSettings":{"network":"xhttp","security":"none","xhttpSettings":{"path":"/trojan-xhttp"}}}]}' "Trojan XHTTP"

# ===================== SUMMARY =====================
echo ""
echo "=========================================="
echo "  PASSED: $PASS   FAILED: $FAIL   TOTAL: $((PASS+FAIL))"
echo "=========================================="
