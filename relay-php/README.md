# relay-php

Official PHP SDK for [Relay](https://github.com/DarkNautica/Relay) — a real-time WebSocket server.

## Installation

```bash
composer require relayhq/relay-php
```

The package auto-discovers the service provider in Laravel.

## Configuration

Publish the config file:

```bash
php artisan vendor:publish --tag=relay-config
```

### Environment Variables

Add these to your `.env`:

```dotenv
RELAY_HOST=127.0.0.1
RELAY_PORT=6001
RELAY_APP_ID=my-app
RELAY_APP_KEY=my-key
RELAY_APP_SECRET=my-secret
```

### Setting the Broadcast Driver

**Laravel 11+** uses `BROADCAST_CONNECTION` in your `.env`:

```dotenv
BROADCAST_CONNECTION=relay
```

**Laravel 10 and below** uses `BROADCAST_DRIVER`:

```dotenv
BROADCAST_DRIVER=relay
```

Then add a `relay` connection in `config/broadcasting.php`:

```php
'connections' => [
    'relay' => [
        'driver'  => 'relay',
        'host'    => env('RELAY_HOST', '127.0.0.1'),
        'port'    => env('RELAY_PORT', 6001),
        'key'     => env('RELAY_APP_KEY'),
        'secret'  => env('RELAY_APP_SECRET'),
        'app_id'  => env('RELAY_APP_ID', 'my-app'),
    ],
],
```

### CSRF Exemption (Required)

The `/broadcasting/auth` endpoint must be excluded from CSRF verification, otherwise private and presence channel authentication will fail with a 419 error.

**Laravel 11+** — in `bootstrap/app.php`:

```php
->withMiddleware(function (Middleware $middleware) {
    $middleware->validateCsrfTokens(except: [
        'broadcasting/auth',
    ]);
})
```

**Laravel 10 and below** — in `app/Http/Middleware/VerifyCsrfToken.php`:

```php
protected $except = [
    'broadcasting/auth',
];
```

## Usage

### Publishing Events

```php
// Using Laravel's broadcast system
broadcast(new MessageSent($message));
```

### Direct Client Usage

```php
use RelayHQ\Relay\RelayClient;

$client = new RelayClient([
    'host'   => '127.0.0.1',
    'port'   => 6001,
    'app_id' => 'my-app',
    'key'    => 'my-key',
    'secret' => 'my-secret',
]);

$client->publish('chat', 'new-message', ['text' => 'Hello!']);
```

## License

MIT
