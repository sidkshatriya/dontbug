/*
 * Copyright 2016 Sidharth Kshatriya
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#ifdef HAVE_CONFIG_H
#include "config.h"
#endif

#include "php.h"
#include "php_ini.h"
#include "ext/standard/info.h"
#include "zend_extensions.h"

#include "php_dontbug.h"

PHP_MINIT_FUNCTION(dontbug) {
    // All opcodes are processed by our user opcode handler
    for (int i = 0; i < 256; i++) {
        zend_set_user_opcode_handler(i, dontbug_common_user_opcode_handler);
    }
    return SUCCESS;
}

PHP_MSHUTDOWN_FUNCTION(dontbug) {
    // Restore to default opcode handler
    for (int i = 0; i < 256; i++) {
        zend_set_user_opcode_handler(i, NULL);
    }
    return SUCCESS;
}

PHP_RINIT_FUNCTION(dontbug) {
#if defined(COMPILE_DL_DONTBUG) && defined(ZTS)
    ZEND_TSRMLS_CACHE_UPDATE();
#endif
    return SUCCESS;
}

PHP_RSHUTDOWN_FUNCTION(dontbug) {
    return SUCCESS;
}

PHP_MINFO_FUNCTION(dontbug) {
    php_info_print_table_start();
    php_info_print_table_row(2, "Dontbug reversible debugger", "enabled");
    php_info_print_table_row(2, "version", PHP_DONTBUG_VERSION);
    // @TODO add freshness information -- i.e. when the module was generated
    php_info_print_table_end();
}

const zend_function_entry dontbug_functions[] = {
        PHP_FE_END };

zend_module_entry dontbug_module_entry = {
        STANDARD_MODULE_HEADER, "dontbug", dontbug_functions,
        PHP_MINIT(dontbug),
        PHP_MSHUTDOWN(dontbug),
        PHP_RINIT(dontbug),
        PHP_RSHUTDOWN(dontbug),
        PHP_MINFO(dontbug),
        PHP_DONTBUG_VERSION,
        STANDARD_MODULE_PROPERTIES };

int dontbug_common_user_opcode_handler(zend_execute_data *execute_data) {
    static char old_location[PHP_DONTBUG_MAX_PATH_LEN];

    zend_op_array *op_array = &execute_data->func->op_array;
    int lineno = execute_data->opline->lineno;

    // @TODO probably need to deal with no filename case better
    char *filename =
            op_array->filename ?
                    ZSTR_VAL(op_array->filename) :
                    "dontbug_couldnt_find_filename";

    char location[PHP_DONTBUG_MAX_PATH_LEN];
    snprintf(location, sizeof(location), "%s:%d", filename, lineno);

    if (strncmp(old_location, location, PHP_DONTBUG_MAX_PATH_LEN) != 0) {
        int ret = dontbug_break_location(op_array->filename, execute_data, lineno);
        strncpy(old_location, location, PHP_DONTBUG_MAX_PATH_LEN);
        return ret;
    } else {
        // same line and file
        return ZEND_USER_OPCODE_DISPATCH;
    }
}

int dontbug_stdout(const char* str, size_t str_len) {
    fwrite(str, 1, str_len, stderr);
    return str_len;
}

void dontbug_eval(char *evalstring) {
    zval zv;
    zend_eval_stringl(evalstring, strlen(evalstring), &zv, "code to eval");
    zend_print_zval_r_ex(dontbug_stdout, &zv, 2);
    fwrite("\n", 1, 1, stderr);
}

ZEND_DLEXPORT int dontbug_zend_startup(zend_extension *extension) {
    return zend_startup_module(&dontbug_module_entry);
}

ZEND_DLEXPORT void dontbug_zend_shutdown(zend_extension *extension) {
}

#ifdef COMPILE_DL_DONTBUG
#ifdef ZTS
ZEND_TSRMLS_CACHE_DEFINE()
#endif
ZEND_GET_MODULE(dontbug)
#endif

#ifndef ZEND_EXT_API
#define ZEND_EXT_API    ZEND_DLEXPORT
#endif
ZEND_EXTENSION();

ZEND_DLEXPORT zend_extension zend_extension_entry = { "dontbug",
        PHP_DONTBUG_VERSION, "(c) 2016", "FAQ", "Sidharth Kshatriya",
        dontbug_zend_startup, dontbug_zend_shutdown,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
        STANDARD_ZEND_EXTENSION_PROPERTIES };

