# Code example

Below is a code block that will be filled with [hello-world.go](./assets/helloworld.go) contents on running ohitmpl. We use the standard go-present notation using sed-like search terms to select which part of the file we wish to show `<!-- .code file.go /start/,/end/ -->`. All lines that have the `omit` keyword are omitted. `omit` must be separated by spaces or a newline.

<!-- .code assets/helloworld.go /package/,/end/ -->

And below we show that extracting only a single line of the file is easy too. 

<!-- .code assets/helloworld.go /fmt/ -->

Above is generated with `<!-- .code assets/helloworld.go /fmt/ -->`

ohimark uses a simple markdown parser so you can safely embed html code without having it escape:
```html
<!-- .code assets/helloworld.go /fmt/ -->
```

.code assets/helloworld.go /fmt/

