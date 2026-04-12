<?php

namespace RelayHQ\Relay;

use Illuminate\Broadcasting\BroadcastManager;
use Illuminate\Support\ServiceProvider;
use RelayHQ\Relay\Broadcasting\RelayBroadcaster;

class RelayServiceProvider extends ServiceProvider
{
    /**
     * Register any application services.
     */
    public function register(): void
    {
        $this->mergeConfigFrom(
            __DIR__ . '/../config/relay.php',
            'relay'
        );
    }

    /**
     * Bootstrap any application services.
     * This hooks Relay into Laravel's Broadcasting system.
     */
    public function boot(): void
    {
        // Publish the config file
        $this->publishes([
            __DIR__ . '/../config/relay.php' => config_path('relay.php'),
        ], 'relay-config');

        // Register the "relay" broadcast driver with Laravel
        $this->app->resolving(BroadcastManager::class, function (BroadcastManager $manager) {
            $manager->extend('relay', function ($app) {
                return new RelayBroadcaster(
                    new RelayClient($app['config']['relay']),
                    $app['config']['relay']
                );
            });
        });
    }
}
