# logrus-datadog-hook

Shipping log entries from [logrus](https://github.com/sirupsen/logrus) to Datadog log API [HTTP endpoint](https://docs.datadoghq.com/api/?lang=bash#send-logs-over-http)

## Example

```golang
    hostName, _ := os.Hostname()
    // Sending log in JSON, batch log every 5 sec and when failure, retry up to 3 times
    hook := NewHook(host, apiKey, 5*time.Second, 3, logrus.TraceLevel, &logrus.JSONFormatter{}, Options{Hostname: hostName})
    l := logrus.New()
    l.Hooks.Add(hook)
    l.WithField("from", "unitest").Infof("TestSendingJSON - %d", i)
```
