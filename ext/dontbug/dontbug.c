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

#include "xdebug/php_xdebug.h"
#include "xdebug/xdebug_str.h"
#include "xdebug/xdebug_var.h"
#include "xdebug/xdebug_handlers.h"
#include "xdebug/xdebug_handler_dbgp.h"

#include "php_dontbug.h"

extern ZEND_DECLARE_MODULE_GLOBALS(xdebug)

PHP_MINIT_FUNCTION(dontbug) {
    return SUCCESS;
}

PHP_MSHUTDOWN_FUNCTION(dontbug) {
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


void dontbug_statement_handler(zend_op_array *op_array) {
    zend_execute_data* execute_data = EG(current_execute_data);

    if (!execute_data) {
        return;
    }

    if (ZEND_USER_CODE(execute_data->func->type) && op_array->filename) {
        // Here just for gdb purposes
        char *filename = ZSTR_VAL(op_array->filename);
        // php line number
        int lineno = execute_data->opline->lineno;

        // stack depth
        unsigned long level = XG(level);

        // level related breakpoints
        dontbug_level_location(level, filename, lineno);

        // Pass the zend_string and not the cstring
        dontbug_break_location(op_array->filename, execute_data, lineno, level);

        return;  // master breakpoint position
    }
}

static char* dontbug_xml_cstringify(xdebug_xml_node *node) {
    xdebug_str *node_xstringified;
    xdebug_str_ptr_init(node_xstringified);

    // Convert the xml to a xdebug_str
    xdebug_xml_return_node(node, node_xstringified);

    // Pass out the c string
    // We don't worry about a memory leak as this is going to be called in a diversion session anyways
    return node_xstringified->d;
}

// Note: this function is always called from GDB
// - This is also why this function is extern
// - Additionally, this function is never called by any other function in this Zend extension
//
// This function will be run in a diversion session from gdb+rr via gdb/mi
// It executes an xdebug command and returns its xml string representation
//
// Parameter "command" is a null-terminated string e.g. "stack_get -i 10"
char* dontbug_xdebug_cmd(char* command) {
    if (!command || strlen(command) < 1) {
        exit(100); // @TODO needs to be filled out. Send a standard error xml node??
    }

    // Outer wrapper <reponse></response>
    xdebug_xml_node *wrapper_node = xdebug_xml_node_init("response");

    // Our context is the current global context XG(context) in the recorded trace in rr
    // This object should be consistent even though we are calling it in a diversion session
    // Its value is what the context object would have held at _that_ point in the replay
    // The locked in meta-data in XG(context) should allow this function to run properly
    int exit_code = xdebug_dbgp_parse_option(&XG(context), command, 0, wrapper_node);

    // Extra attributes
    xdebug_xml_add_attribute(wrapper_node, "xmlns", "urn:debugger_protocol_v1");
    xdebug_xml_add_attribute(wrapper_node, "xmlns:xdebug", "http://xdebug.org/dbgp/xdebug");

    if (exit_code != 1) {
        // Return a string representation of the xml back to gdb
        // We don't worry about a memory leak as the forked process is going to be
        // terminated eventually
        return dontbug_xml_cstringify(wrapper_node);
    }

    exit(100); // @TODO needs to be filled out. Send a standard error xml node??
}

ZEND_DLEXPORT int dontbug_zend_startup(zend_extension *extension) {
    // @TODO check if xdebug zend extension is enabled as dontbug needs it

    // This specific string is searched for by the dontbug engine
    // DONT CHANGE IT!
    fprintf(stderr, "Successfully loaded dontbug.so\n");
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
        dontbug_statement_handler,  // typedef void (*statement_handler_func_t)(zend_op_array *op_array);
        NULL,
        NULL,
        NULL,
        NULL,
        STANDARD_ZEND_EXTENSION_PROPERTIES };

