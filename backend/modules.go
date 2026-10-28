package main

import (
	_ "godeb/plugins/apt"
	_ "godeb/plugins/certbot"
	_ "godeb/plugins/cron"
	_ "godeb/plugins/domains"
	_ "godeb/plugins/fail2ban"
	_ "godeb/plugins/logs"
	_ "godeb/plugins/mail"
	_ "godeb/plugins/mailgateway/backend"
	_ "godeb/plugins/monitor"
	_ "godeb/plugins/portforward"
	_ "godeb/plugins/postgres"
	_ "godeb/plugins/steam"
	_ "godeb/plugins/systemd"
	_ "godeb/plugins/ufw"
	_ "godeb/plugins/websites"
	_ "godeb/plugins/wireguard"
)
