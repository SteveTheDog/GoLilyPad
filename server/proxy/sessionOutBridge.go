package proxy

import (
	"bytes"
	"net"
	"time"
	"strconv"
	"github.com/LilyPad/GoLilyPad/packet"
	"github.com/LilyPad/GoLilyPad/packet/minecraft"
	"github.com/LilyPad/GoLilyPad/server/proxy/connect"
)

type SessionOutBridge struct {
	session *Session
	server *connect.Server
	conn net.Conn
	connCodec *packet.PacketConnCodec
	codec *packet.PacketCodecVariable

	remoteIp string
	remotePort string
	state SessionState
}

func NewSessionOutBridge(session *Session, server *connect.Server, conn net.Conn) (this *SessionOutBridge) {
	this = new(SessionOutBridge)
	this.session = session
	this.server = server
	this.conn = conn
	this.remoteIp, this.remotePort, _ = net.SplitHostPort(conn.RemoteAddr().String())
	this.state = STATE_DISCONNECTED
	return
}

func (this *SessionOutBridge) Serve() {
	this.codec = packet.NewPacketCodecVariable(minecraft.HandshakePacketServerCodec, minecraft.HandshakePacketClientCodec)
	this.connCodec = packet.NewPacketConnCodec(this.conn, this.codec, 30 * time.Second)

	inRemotePort, _ := strconv.ParseUint(this.session.remotePort, 10, 16)
	outRemotePort, _ := strconv.ParseUint(this.remotePort, 10, 16)
	loginPayload := LoginPayload{
		SecurityKey: this.server.SecurityKey,
		Host: this.session.serverAddress,
		RealIp: this.session.remoteIp,
		RealPort: int(inRemotePort),
		Name: this.session.name,
		UUID: this.session.profile.Id,
		Properties: make([]LoginPayloadProperty, 0),
	}
	for _, property := range this.session.profile.Properties {
		loginPayload.Properties = append(loginPayload.Properties, LoginPayloadProperty{property.Name, property.Value, property.Signature})
	}
	this.Write(minecraft.NewPacketServerHandshake(this.session.protocolVersion, EncodeLoginPayload(loginPayload), uint16(outRemotePort), 2))

	this.codec.SetEncodeCodec(minecraft.LoginPacketServerCodec)
	this.codec.SetDecodeCodec(minecraft.LoginPacketClientCodec)
	this.Write(minecraft.NewPacketServerLoginStart(this.session.name))

	this.state = STATE_LOGIN
	go this.connCodec.ReadConn(this)
}

func (this *SessionOutBridge) Write(packet packet.Packet) (err error) {
	err = this.connCodec.Write(packet)
	return
}

func (this *SessionOutBridge) HandlePacket(packet packet.Packet) (err error) {
	if !this.session.Authenticated() {
		this.conn.Close()
		return
	}
	switch this.state {
	case STATE_LOGIN:
		if packet.Id() == minecraft.PACKET_CLIENT_LOGIN_SUCCESS {
			this.session.redirectMutex.Lock()
			this.state = STATE_INIT
			this.session.redirecting = true
			this.codec.SetEncodeCodec(minecraft.PlayPacketServerCodec)
			this.codec.SetDecodeCodec(minecraft.PlayPacketClientCodec)
		} else if packet.Id() == minecraft.PACKET_CLIENT_LOGIN_DISCONNECT {
			this.session.DisconnectJson(packet.(*minecraft.PacketClientLoginDisconnect).Json)
			this.conn.Close()
		} else {
			if this.session.Initializing() {
				this.session.Disconnect("Error: Outbound Protocol Mismatch")
			}
			this.conn.Close()
		}
	case STATE_INIT:
		if packet.Id() == minecraft.PACKET_CLIENT_PLAYER_POSITION_AND_LOOK {
			this.session.outBridge = this
			this.session.redirecting = false
			this.session.state = STATE_CONNECTED
			this.state = STATE_CONNECTED
			this.session.redirectMutex.Unlock()
		}
		fallthrough
	case STATE_CONNECTED:
		if packet.Id() == minecraft.PACKET_CLIENT_DISCONNECT {
			this.state = STATE_DISCONNECTED
		}
		if this.state == STATE_CONNECTED {
			this.session.redirectMutex.Lock()
			if this.session.outBridge != this {
				this.conn.Close()
				this.session.redirectMutex.Unlock()
				break
			}
			this.session.redirectMutex.Unlock()
		}
		switch packet.Id() {
		case minecraft.PACKET_CLIENT_JOIN_GAME:
			joinGamePacket := packet.(*minecraft.PacketClientJoinGame)
			if this.session.clientSettings != nil {
				this.Write(this.session.clientSettings)
			}
			this.session.serverEntityId = joinGamePacket.EntityId
			if this.session.state == STATE_INIT {
				this.session.clientEntityId = joinGamePacket.EntityId
			} else {
				var swapDimension int32
				if joinGamePacket.Dimension == 0 {
					swapDimension = 1
				} else {
					swapDimension = 0
				}
				this.session.Write(minecraft.NewPacketClientRespawn(swapDimension, 2, 0, "DEFAULT"))
				this.session.Write(minecraft.NewPacketClientRespawn(int32(joinGamePacket.Dimension), joinGamePacket.Difficulty, joinGamePacket.Gamemode, joinGamePacket.LevelType))
				for player, _ := range this.session.playerList {
					this.session.Write(minecraft.NewPacketClientPlayerListItemRemove(player))
				}
				this.session.playerList = make(map[string]struct{})
				for scoreboard, _ := range this.session.scoreboards {
					this.session.Write(minecraft.NewPacketClientScoreboardObjectiveRemove(scoreboard, ""))
				}
				this.session.scoreboards = make(map[string]struct{})
				for team, _ := range this.session.teams {
					this.session.Write(minecraft.NewPacketClientTeamsRemove(team))
				}
				this.session.teams = make(map[string]struct{})
				if len(this.session.pluginChannels) > 0 {
					channels := make([][]byte, len(this.session.pluginChannels))
					for channel, _ := range this.session.pluginChannels {
						channels = append(channels, []byte(channel))
					}
					this.Write(minecraft.NewPacketServerPluginMessage("REGISTER", bytes.Join(channels, []byte{0})))
				}
				return
			}
		case minecraft.PACKET_CLIENT_PLAYER_LIST_ITEM:
			playerListPacket := packet.(*minecraft.PacketClientPlayerListItem)
			if playerListPacket.Online {
				this.session.playerList[playerListPacket.Name] = struct{}{}
			} else {
				delete(this.session.playerList, playerListPacket.Name)
			}
		case minecraft.PACKET_CLIENT_SCOREBOARD_OBJECTIVE:
			scoreboardPacket := packet.(*minecraft.PacketClientScoreboardObjective)
			if scoreboardPacket.Action == 0 {
				this.session.scoreboards[scoreboardPacket.Name] = struct{}{}
			} else if scoreboardPacket.Action == 1 {
				delete(this.session.scoreboards, scoreboardPacket.Name)
			}
		case minecraft.PACKET_CLIENT_TEAMS:
			teamPacket := packet.(*minecraft.PacketClientTeams)
			if teamPacket.Action == 0 {
				this.session.teams[teamPacket.Name] = struct{}{}
			} else if teamPacket.Action == 1 {
				delete(this.session.teams, teamPacket.Name)
			}
		case minecraft.PACKET_CLIENT_DISCONNECT:
			this.session.DisconnectJson(packet.(*minecraft.PacketClientDisconnect).Json)
			return
		default:
			if genericPacket, ok := packet.(*minecraft.PacketGeneric); ok {
				genericPacket.SwapEntities(this.session.clientEntityId, this.session.serverEntityId, true)
			}
		}
		this.session.Write(packet)
	}
	return
}

func (this *SessionOutBridge) ErrorCaught(err error) {
	if this.state == STATE_INIT {
		this.session.redirecting = false
		this.session.redirectMutex.Unlock()
	}
	if this.state != STATE_DISCONNECTED && this.session.outBridge == this {
		this.session.Disconnect(minecraft.Colorize(this.session.server.localizer.LocaleLostConn()))
	}
	this.session = nil
	this.server = nil
	this.state = STATE_DISCONNECTED
	this.conn.Close()
}
