Viam MicroRDK on an ESP32 with a TMP36 analog sensor.
Read sensor push to app.viam.com.

Sample machine config:

```
{
  "services": [],
  "components": [
    {
      "type": "board",
      "model": "esp32",
      "attributes": {
        "pins": [],
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
	"app_api_key" : "topsecret"
}
```
