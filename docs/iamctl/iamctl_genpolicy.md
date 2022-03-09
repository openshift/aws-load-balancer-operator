## iamctl gopolicy

Used to generate AWS IAM Policy from policy json.

### Synopsis

Used for creating/updating iam policies required by the aws loadbalancer operator.

```
iamctl gopolicy [flags]
```

### Options

```
  -h, --help                 help for gopolicy
  -i, --input-file string    Used to specify input JSON file path.
  -o, --output-file string   Used to specify output Go file path.
  -p, --package string       Used to specify output Go file path. (default "main")
```

### SEE ALSO

* [iamctl](iamctl.md)	 - A CLI used to convert aws iam policy JSON to Go code.

