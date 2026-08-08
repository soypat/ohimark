# ohimark
You are tearing me apart, Markdown libraries of the Go ecosystem.

I want something that is lightweight for simple parsing. Have not thought of extensibility yet but am open to breaking changes if the API is intuitive and easy to use.

## Example

```sh
$ go run ./examples/justmarkit -i README.md
document open len: 0
heading open len: 2
text open len: 7
heading close len: 9
paragraph open len: 0
text open len: 65
paragraph close len: 65
paragraph open len: 0
text open len: 167
paragraph close len: 167
heading open len: 3
text open len: 9
heading close len: 12
paragraph open len: 0
text open len: 138
paragraph close len: 138
document close len: 400
2026/08/08 13:45:21 finished 74.653µs
```

## AI notice
This package was designed by a human and assisted by an AI. We follow Oxide's RFD on LLM usage: https://rfd.shared.oxide.computer/rfd/0576 