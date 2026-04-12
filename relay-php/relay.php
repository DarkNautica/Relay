<?php

return [

    /*
    |--------------------------------------------------------------------------
    | Relay Server Connection
    |--------------------------------------------------------------------------
    |
    | The host and port of your Relay server. For local development this is
    | typically localhost:6001. For production, point to your server's IP
    | or domain.
    |
    */

    'host' => env('RELAY_HOST', '127.0.0.1'),
    'port' => env('RELAY_PORT', 6001),
    'tls'  => env('RELAY_TLS', false),

    /*
    |--------------------------------------------------------------------------
    | Application Credentials
    |--------------------------------------------------------------------------
    |
    | These must match the credentials configured on your Relay server.
    | RELAY_APP_KEY is used by the JS client to connect.
    | RELAY_APP_SECRET is used by your backend to publish events.
    |
    */

    'app_id' => env('RELAY_APP_ID', 'my-app'),
    'key'    => env('RELAY_APP_KEY', 'my-key'),
    'secret' => env('RELAY_APP_SECRET', 'my-secret'),

    /*
    |--------------------------------------------------------------------------
    | HTTP Client Options
    |--------------------------------------------------------------------------
    */

    'timeout' => env('RELAY_TIMEOUT', 5),

];
