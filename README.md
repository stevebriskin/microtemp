# Temperature Reading Client

## Description

Viam MicroRDK on an ESP32 with a TMP36 analog sensor. Read sensor and push readings to app.viam.com.

## Configs

Sample app.viam.com machine config:

```
{
  "services": [],
  "components": [
    {
      "type": "board",
      "model": "esp32",
      "attributes": {
        "pins": [12],
        "analogs": [
          {
            "pin": 35,
            "name": "temp"
          }
        ]
      },
      "depends_on": [],
      "name": "board"
    }
  ],
}
```

Sample client script config:

```
{
	"machines" : [
		{
			"part_id": "3910b942-....",
			"part_uri" : "ABC.XYZ.viam.cloud",
			"mach_api_name" : "0570a....",
			"mach_api_key" : "topsecret"
		}
	],
	"app_api_name" : "9600...",
	"app_api_key" : "topsecret",
  "sleep_time" : 180,
  "num_sensor_readings" : 10
}
```
