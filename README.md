# DNS IP module for Caddy

This module retrieves IP addresses from DNS and returns them as single-IP prefixes,
for use in Caddy `trusted_proxies` directives.

## Example config

For example, if you're running [cloudflared](https://hub.docker.com/r/cloudflare/cloudflared)
in a Docker container on the same bridge network as Caddy,
this will look up that container by service name:

```Caddy
trusted_proxies dns cloudflared {
    # Explicitly set the default value.
    interval 1m
}
```

Alternatively, the hostname can be specified inside the block:

```Caddy
trusted_proxies dns {
    host cloudflared
}
```

Multiple hosts can be specified, either on the same line or as separate `host` directives.

```Caddy
trusted_proxies dns {
    host proxy-1.example.com proxy-2.example.com
    host proxy-3.example.com
}
```

You can even mix these, though I would advise against it due to readability:

```Caddy
trusted_proxies dns proxy-1.example.com proxy-2.example.com {
    host proxy-3.example.com
}
```

## Settings

| Name     | Description                                       | Type     | Default                 |
|----------|---------------------------------------------------|----------|-------------------------|
| host     | The host name(s) to look up.                      | string   | N/A, must be specified. |
| interval | How often the IP address(es) should be refreshed. | duration | 1m (every minute)       |
