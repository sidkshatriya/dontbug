PHP_ARG_ENABLE(dontbug, whether to enable dontbug support,[  --enable-dontbug        Enable dontbug support])

if test "$PHP_DONTBUG" != "no"; then
  PHP_NEW_EXTENSION(dontbug, dontbug.c dontbug_break.c, $ext_shared,, -DZEND_ENABLE_STATIC_TSRMLS_CACHE=1)
fi
