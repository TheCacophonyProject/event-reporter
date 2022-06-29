module github.com/TheCacophonyProject/event-reporter/v3

go 1.15

require (
	github.com/TheCacophonyProject/go-api v1.0.2
	github.com/TheCacophonyProject/lora_events v0.0.0-20220628235236-ee6d3e7ef9c8
	github.com/TheCacophonyProject/modemd v0.0.0-20190605010435-ae5b0f2eb760
	github.com/alexflint/go-arg v1.4.2
	github.com/boltdb/bolt v1.3.1
	github.com/coreos/go-systemd v0.0.0-20190321100706-95778dfbb74e
	github.com/godbus/dbus v0.0.0-20181101234600-2ff6f7ffd60f
	github.com/stretchr/testify v1.7.0
)

replace periph.io/x/periph => github.com/TheCacophonyProject/periph v2.1.1-0.20200615222341-6834cd5be8c1+incompatible
