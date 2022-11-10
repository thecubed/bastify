bastify
====

Create a SOCKS5 SSH tunnel dynamically via SOCKS username and password selection of bastion hosts.

Ideally used with `kubectl` to transparently proxy requests to a specific bastion host when managing a large amount of clusters.

## Usage

```
Usage:
  bastify [OPTIONS]

Application Options:
  -l, --listen-host=     SOCKS5 listen host (default: 127.0.0.1)
  -p, --listen-port=     SOCKS5 listen port. (default: 5101)
  -u, --user=            bastion SSH username. Leave blank to use current user name.
  -k, --key-file=        Private key file to use when authenticating with bastion hosts. Leave unset to rely on SSH agent.
  -t, --idle-close=      Idle timeout before closing bastion SSH connection. (default: 4h)
  -r, --max-retries=     Maximum retries for a port forward through a bastion SSH connection (default: 2)
      --status-interval= Display connection statistics on this interval (default: 0)
  -v                     Change logging verbosity

Help Options:
  -h, --help             Show this help message
```

Simply start bastify by running `bastify`!

Once `bastify` is running, you can use it as a transparent SOCKS5 proxy to access any service behind any SSH bastion dynamically.

The URL format for the proxy server is as follows:
```
# change to socks5h:// for cURL remote DNS resolution
socks5://[bastion hostname/ip]:[bastion ssh server port]@localhost:[bastify port]
```

To test, try:
```
curl --proxy socks5h://mybastion:22@localhost:5101 https://somehost
```

## Kubernetes Usage

To use with Kubernetes, simply set your `proxy-url` as per this example:
```
kind: Config
preferences: {}
apiVersion: v1
clusters:
  - cluster:
      certificate-authority-data: ...
      server: https://myapiserver.myprivatedomain.net:6443
      # specify the host that protects the api server here
      proxy-url: socks5://mybastion1:22@localhost:5101/
    name: cluster1
  - cluster:
      certificate-authority-data: ...
      server: https://api.someotherprivatedomain.net:6443
      # specify the host that protects the api server here
      proxy-url: socks5://differentbastion:22@localhost:5101/
    name: cluster2

contexts:
  - context:
      cluster: cluster1
      user: test
    name: cluster1
  - context:
      cluster: cluster2
      user: test
    name: cluster2

current-context: cluster1

users:
  - name: test
    user:
      username: ...
      password: ...
```

As you can see from above, this saves you from having to create multiple SSH tunnels prior to launching `kubectl`, since
as long as you have `bastify` running, your requests will automatically be proxied to the correct bastion host.


## Thanks

Special thanks to [sockssh](https://github.com/getlantern/sockssh) for providing the base upon which this project was born!

## License

Apache License 2.0
