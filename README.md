# root-detector
A go binary to iterate through every container in every pod in every namespace to check if the user is running as root. By default `"kube-system", "kube-public", and "kube-node-lease"` are excluded but you can edit this to your liking. 

## Installing
`go build .`

## Testing
`go test`

## Authentication
This is meant to be run externally from the cluster. It looks for a kube config file in the deafult location of `$HOME/.kube/config`

## License
The source code for the site is licensed under the MIT license, which you can find in the MIT-LICENSE.txt file.
