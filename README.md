# Dontbug Debugger

Dontbug is a reversible debugger for PHP. It allows you to record the execution of PHP scripts (in command line mode or in the browser) and replay the same execution back in a PHP IDE debugger. During replay you may debug normally (forward mode debugging) or in reverse, which allows you to step over/out backwards, step backwards, run backwards, run to cursor backwards, set breakpoints in the past and so forth.

Debugging with the ability to run in reverse allows you to hunt down bugs much more easily. It also allows you to understand the runtime behavior of large PHP codebases more efficiently.

## Debugger features
- Debug PHP sources in forward or reverse
- Ability to set line breakpoints, inspect PHP variables and the call stack, step over/out/into backwards or forward, hit breakpoints when running in reverse or forward mode, run to cursor backwards etc.
- Full compatibility with existing PHP IDEs like [Netbeans](https://netbeans.org/features/php/), [Eclipse PDT](http://www.eclipse.org/pdt/), [PhpStorm](https://www.jetbrains.com/phpstorm/). No special IDE plugins or modifications required for your IDE
- Minimal learning curve: Apart from getting familiar with debugging in reverse, you continue using the same debugger as before. When dontbug is put into reverse mode, the buttons on your IDE simply acquire opposite meanings. So step over is now "step over" backwards. This can be confusing, so [here](#debugging-in-reverse-mode-can-be-confusing-but-here-is-a-cheat-sheet) is a cheat sheet.
- Ability to record PHP script execution completely even if there are network calls, database calls or any non-deterministic input/output in the PHP code. During replay, the PHP scripts will see the _same_ input/output results from databases, network calls, calls to `rand()/time()` etc. as during record. (However, PHP will not write/read to the network or database a second time during replay)
- Highly performant forward/reverse mode execution so you can concentrate on finding the bug and not have the debugger "get in your way"
- Ability to record multiple web-server requests/responses in one go: Traditional PHP (website) debugging is done on a per URL basis. With dontbug you can record many webserver requests/responses at a time and then debug the consolidated execution trace. This can help you hit breakpoints in code which are rarely triggered or triggered in poorly understood situations. What this means in practice is that pressing run/continue (in forward or reverse mode) can often lead you to the next/previous request in the debugger and not the end of the program. (Feature caveat: be aware that recording too many page requests/responses at a time may degrade performance when debugging)

## Limitations and Caveats
Since dontbug replays a saved PHP script execution trace, you cannot persistently modify a variable value in the debugger. All variables (and "state") in the PHP script is read-only. This limitation is fundamental in the current record/replay architecture. In practice, this is not such a big limitation as changing variable values debugging is rarely needed. 

Dontbug is pre-alpha quality. It works but is a bit rough around the edges. Please report any problems you encounter. Dontbug also does not have advanced debugging features like breaking on named exceptions, breaking on call to/return from a specifically named function, conditional breakpoints[*], watches etc. at the moment. Some of these are planned for future releases.

[*] You can always emulate conditional breakpoints by adding an `if` statement for the breakpoint condition and a line breakpoint inside the `if` statement.

## Usage in Brief
- Record an execution by using `dontbug record`
- Ask your PHP IDE to listen for a debugging connection
- In your favorite shell, execute: `dontbug replay`
- Dontbug now tries to connect to the PHP IDE that is listening for debugger connections
- Once connected, dontbug will replay the last execution recorded (via `dontbug record`) to the IDE
- Once connected, use the debugger in the IDE as you would, normally
- If you want run in reverse mode, press "r" for reverse mode and "f" for forward mode in the dontbug prompt

See below for more details.

## Installation and OS Platforms
See [Installation Instructions](https://github.com/sidkshatriya/dontbug/wiki/Installation-Instructions) on installing Dontbug on your machine.

## How the Dontbug Debugger Works

If you're interested in how the Dontbug debugger works internally please read [this](https://github.com/sidkshatriya/dontbug/wiki/How-the-Dontbug-Debugger-works) document.

## Credits
_Standing on the shoulders of giants_

Most open source projects are built with the help of other open source projects and Dontbug is no exception.

The [Mozilla/RR](http://rr-project.org/), [Xdebug](https://xdebug.org/) and [GDB](https://www.gnu.org/software/gdb/) projects are the most important open source projects that Dontbug depends on. The existence of these fantastic open source projects has allowed Dontbug to be built. The roles and contributions of these projects to the overall functionality of Dontbug is outlined in the ["How the Dontbug Debugger works"](https://github.com/sidkshatriya/dontbug/wiki/How-the-Dontbug-Debugger-works) document.

The Dontbug executable has been programmed in the [golang](https://golang.org) programming language. It uses a number of open source golang libraries. Please consult the source code to know the specific ones used.

## License and Copyright

Dontbug is Copyright © 2016 Sidharth Kshatriya. Dontbug is licensed under the terms of the Apache License (Version 2.0). 

# Using the Dontbug Debugger
Once you have installed `dontbug` you have two commands available: 
- `dontbug record`
- `dontbug replay` 

See `dontbug record --help` or `dontbug replay --help` for more information or continue reading.

## Command `dontbug record`
The `dontbug record` command records the execution of PHP scripts in the [PHP built-in webserver](https://secure.php.net/manual/en/features.commandline.webserver.php) or in the PHP command line interpreter. This is used for later forward/reverse debugging in a PHP IDE. A typical workflow is to do a `dontbug record` followed by `dontbug replay`.

### Syntax
    dontbug record <php-source-root-dir> [<docroot-dir>] [flags]
    dontbug record <php-source-root-dir> <php-script> --php-cli-script [args-in-quotes] [flags]

### Examples
    dontbug record /var/www/fancy-site docroot
    dontbug record /var/www/another-site

    dontbug record ~/php-test/ list-supported-functions.php --php-cli-script
    dontbug record ~/php-test/ math/calculate-factorial-min-max.php --php-cli-script --args "10 20"

The first example will spawn the PHP built-in webserver for recording the execution of "fancy-site" website (as the user navigates various URLs in a browser). The docroot of the fancy site will be `/var/www/fancy-site/docroot` and the `<php-source-dir>` will be `/var/www/fancy-site`.

In general, dontbug will be able to handle any PHP framework/CMS as long you meet its minimum requirements and the framework/CMS runs in PHP's built in webserver (most of them should). Here the PHP built in webserver is substituting for Apache, Nginx etc.

The second example is like the first. Here the `<docroot-dir>` is assumed to be the same as the `<php-source-root-dir>`.

The third example will record the execution of `~/php-test/list-supported-functions.php`

The fourth example will record the execution of a PHP script with two arguments 10 and 20 passed to it. Note the quotes to enclose the arguments. The script's full path is `~/php-test/math/calculate-factorial-min-max.php`.

As you have seen _if_ you specify `<docroot-dir>` or `<php-script>` then it should specified as a _relative_ path w.r.t to the `<php-source-root-dir>`.

The `<php-source-root-dir>` means the outermost directory of all possible PHP scripts that might be executed in this project by PHP sources in this project.

#### Note
- Typically `<php-source-root-dir>` would be the docroot in your PHP project or, sometimes its parent folder. `<php-source-root-dir>` is _not_ the same as `<docroot-dir>`, sometimes, as scripts might be placed outside the docroot in some PHP projects e.g. vendor scripts installed by composer. Please keep this directory as specific as possible. For example, you _could_ specify "/" (the root directory) as `<php-source-root-dir>` as it contains all the possible PHP scripts on your system. But this would impact performance hugely.
- If you have sources symlinked from inside the `<php-source-root-dir>` to outside that dir, dontbug should be able to handle that (without you having to increase the scope of the `<php-source-root-dir>`)

### PHP built-in webserver tips
You may record as many http page loads for later debugging when running the PHP built in webserver (unlike traditional PHP debugging which is usually one page load at a time). **However be aware that recording too many page loads may degrade performance during debugging**. Additionally, you may _not_ pass arguments to the PHP built in server i.e. the `--args` flag is ignored if not used in conjunction with `--php-cli-script`.

### Config file
If you find that you are frequently passing the same flags to `dontbug`, you may provide custom config for various flags in a `$HOME/.dontbug.yaml` file. Sample file:

```
server-port: 8003
install-location: /some-path/src/github.com/sidkshatriya/dontbug
```

Most of the parameters defaults should suffice and you will typically need a very minimal `.dontbug.yaml` config file.

Flags passed via command line will always override any configuration in a `.yaml` file. If the `.yaml` file and user flags don't specify a particular parameter, the defaults mentioned in `dontbug record --help` will apply.

### More information and flags
See `dontbug record --help` for more information on the various flags available for more customization options

## Command `dontbug replay`
The `dontbug replay` command replays a previously saved execution trace to a PHP IDE debugger. You may set breakpoints, step through code, inspect variable values etc. as you are used to. But more interestingly, dontbug allows you to _reverse_ debug i.e. step over backwards, run backwards, hit breakpoints when running in reverse and so forth.

dontbug communicates with PHP IDEs by using the [dbgp](https://xdebug.org/docs-dbgp.php) protocol which is the defacto standard for PHP IDEs so _no special support_ is required for dontbug to work with them. As far as the IDEs are concerned they are talking with a normal PHP debug engine.

### Basic Usage
- Record an execution by using `dontbug record` (see `dontbug record --help` to know how to do this)
- Ask your PHP IDE to listen for a debugging connection
- In your favorite shell, execute: `dontbug replay`
- Dontbug now tries to connect to the PHP IDE that is listening for debugger connections
- Once connected, dontbug will replay the last execution recorded (via `dontbug record`) to the IDE
- Once connected, use the debugger in the IDE as you would, normally
- If you want run in reverse mode, press "r" for reverse mode and "f" for forward mode in the dontbug prompt. In reverse mode the buttons in your IDE will remain the same but they will have the reverse effect when you press them: e.g. Step Over will now be reverse Step Over and so forth
- Press h for help on dontbug prompt for more information

### Tips, Gotchas
**Some PHP IDEs will try to open a browser window when they start listening for debug connections**. Let them do that. The URL they access in the browser is likely to result in an error anyways. **Ignore the error**. This has absolutely no effect on dontbug as we're replaying a previously saved execution trace but the IDE does not know that.

The only important thing is to look for a message in green "dontbug: Connected to PHP IDE debugger" on the dontbug prompt. Once you see this message, you can start debugging in your PHP IDE as you normally would. Except you now have the ability to run in reverse when you want.

**Remember that replays are frozen in time.** If you change your PHP code after a replay, you will need to do `dontbug record` again. If you don't do that `dontbug replay` will replay a PHP execution corresponding to an **earlier version** of the source code while your PHP IDE will show the current PHP source code! This can lead to a lot of confusion (and weird behavior) as you may imagine. 

Essentially, you need make sure that your PHP sources are _not_ newer than the last recording. If they are, you should simply `dontbug record` again. There is an advanced/experimental flag `--take-snapshot` (see `dontbug record --help`) which allows you to record an execution _and_ take a source code snapshot so that the above issue can be dealt with in a more principled way. However, this feature is not fully documented yet and increases the complexity of your workflow. Therefore: simply do a `dontbug record` again if your PHP sources have changed since the last recording!

### More information and flags
See `dontbug replay --help` for more information on the various flags available for more customization options

## dontbug prompt
Upon running `dontbug replay` you have a prompt available in which you can switch between forward and reverse modes. This prompt also has help. Press `h` followed by `<enter>` to get help.

```
(dontbug) h
h        display this help text
q        quit
r        debug in reverse mode
f        debug in forward (normal) mode
t        toggle between reverse and forward modes
v        toggle between verbose and quiet modes
n        toggle between showing and not showing gdb notifications
<enter>  will tell you whether you are in forward or reverse mode
```

### Debugging in reverse mode can be confusing but here is a cheat sheet
The buttons in your PHP IDE debugger will have the following new (and opposite) meanings in reverse debugging mode:

- Step Into now means "Step Into a PHP statement in the reverse direction"
- Step Over now means "Step Over one PHP statement backwards. As usual, stop if you encounter a breakpoint while doing this operation"
- Step Out now means "Run backwards until you come out of the current function and are about to enter it. As usual, stop if you encounter a breakpoint while doing this operation"
- Run/Continue  now means "Run backwards until you hit a breakpoint"
- Run to Cursor now means "Run backwards until you hit the cursor (need to place cursor before current line)"
