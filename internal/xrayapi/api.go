package xrayapi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/xtls/xray-core/app/proxyman/command"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"
	"github.com/xtls/xray-core/proxy/vmess"
)

func getApiServerAddress() string {
	data, err := os.ReadFile("/etc/xray/config.json")
	if err != nil {
		return "127.0.0.1:10085"
	}
	var cfg struct {
		Inbounds []struct {
			Listen   string `json:"listen"`
			Port     int    `json:"port"`
			Protocol string `json:"protocol"`
			Tag      string `json:"tag"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "127.0.0.1:10085"
	}
	for _, ib := range cfg.Inbounds {
		if ib.Tag == "api" {
			addr := ib.Listen
			if addr == "" {
				addr = "127.0.0.1"
			}
			return fmt.Sprintf("%s:%d", addr, ib.Port)
		}
	}
	return "127.0.0.1:10085"
}

type InboundInfo struct {
	Protocol       string `json:"protocol"`
	Tag            string `json:"tag"`
	StreamSettings struct {
		Network  string `json:"network"`
		Security string `json:"security"`
	} `json:"streamSettings"`
}

func getInboundByTag(tag string) (*InboundInfo, error) {
	data, err := os.ReadFile("/etc/xray/config.json")
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Inbounds []InboundInfo `json:"inbounds"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	for _, ib := range cfg.Inbounds {
		if ib.Tag == tag {
			return &ib, nil
		}
	}
	return nil, fmt.Errorf("inbound tag %s not found", tag)
}

// AddUserToInbound dynamically injects a user into a specific Xray inbound
func AddUserToInbound(inboundTag string, protocolType string, email string, uuidStr string) error {
	apiAddr := getApiServerAddress()
	conn, err := grpc.NewClient(apiAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	client := command.NewHandlerServiceClient(conn)

	var accountMsg *serial.TypedMessage

	switch protocolType {
	case "vless", "vless-reality":
		flow := ""
		ib, err := getInboundByTag(inboundTag)
		if err == nil {
			if ib.StreamSettings.Security == "reality" && inboundTag == "inbound-vless-10444" {
				flow = "xtls-rprx-vision"
			}
		}
		accountMsg = serial.ToTypedMessage(&vless.Account{
			Id:   uuidStr,
			Flow: flow,
		})
	case "vmess":
		accountMsg = serial.ToTypedMessage(&vmess.Account{
			Id: uuidStr,
		})
	case "trojan":
		accountMsg = serial.ToTypedMessage(&trojan.Account{
			Password: uuidStr,
		})
	default:
		return fmt.Errorf("unsupported protocol: %s", protocolType)
	}

	user := &protocol.User{
		Level:   0,
		Email:   email,
		Account: accountMsg,
	}

	req := &command.AlterInboundRequest{
		Tag: inboundTag,
		Operation: serial.ToTypedMessage(&command.AddUserOperation{
			User: user,
		}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = client.AlterInbound(ctx, req)
	return err
}

// RemoveUserFromInbound dynamically removes a user from an Xray inbound
func RemoveUserFromInbound(inboundTag string, email string) error {
	apiAddr := getApiServerAddress()
	conn, err := grpc.NewClient(apiAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	client := command.NewHandlerServiceClient(conn)

	req := &command.AlterInboundRequest{
		Tag: inboundTag,
		Operation: serial.ToTypedMessage(&command.RemoveUserOperation{
			Email: email,
		}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = client.AlterInbound(ctx, req)
	return err
}

type XrayConfig struct {
	Inbounds []InboundInfo `json:"inbounds"`
}

// GetInboundsByProtocol reads the local config.json and returns all inbound tags matching the protocol.
func GetInboundsByProtocol(protocol string) ([]string, []string, error) {
	data, err := os.ReadFile("/etc/xray/config.json")
	if err != nil {
		return nil, nil, err
	}
	var config XrayConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, nil, err
	}

	var standardTags []string
	var realityTags []string

	for _, ib := range config.Inbounds {
		if ib.Protocol == protocol && ib.Tag != "api" && ib.Tag != "" {
			if ib.StreamSettings.Security == "reality" {
				realityTags = append(realityTags, ib.Tag)
			} else {
				standardTags = append(standardTags, ib.Tag)
			}
		}
	}
	return standardTags, realityTags, nil
}

