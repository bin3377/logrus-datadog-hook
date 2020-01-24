# logrus-datadog-hook

Shipping log entries from [logrus](https://github.com/sirupsen/logrus) to Datadog log API [HTTP endpoint](https://docs.datadoghq.com/api/?lang=bash#send-logs-over-http)

## Example

```golang
    // Sending log in JSON format
    hostName, _ := os.Hostname()
    // When failure, retry up to 3 times with 5s interval
    hook := datadog.NewHook(datadog.DatadogUSHost, apiKey, true, 3, 5*time.Second)
    hook.Hostname = hostName
    l := logrus.New()
    l.Hooks.Add(hook)
    l.WithField("from", "unitest").Infof("TestSendingJSON - %d", i)
```

```golang
    // Sending log in plain text
    hostName, _ := os.Hostname()
    // When failure, retry up to 3 times with 5s interval
    hook := datadog.NewHook(datadog.DatadogUSHost, apiKey, false, 3, 5*time.Second)
    hook.Hostname = hostName
    l := logrus.New()
    l.Hooks.Add(hook)
    l.WithField("from", "unitest").Infof("TestSendingText - %d", i)
```
