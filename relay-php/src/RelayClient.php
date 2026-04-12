<?php

namespace RelayHQ\Relay;

use GuzzleHttp\Client as HttpClient;
use GuzzleHttp\Exception\GuzzleException;
use RelayHQ\Relay\Exceptions\RelayException;

/**
 * RelayClient handles all HTTP communication with the Relay server.
 * This is what your Laravel app uses to publish events server-side.
 */
class RelayClient
{
    private HttpClient $http;
    private array $config;

    public function __construct(array $config)
    {
        $this->config = $config;
        $this->http = new HttpClient([
            'base_uri' => $this->buildBaseUrl(),
            'timeout'  => $config['timeout'] ?? 5,
            'headers'  => [
                'Authorization' => 'Bearer ' . $config['secret'],
                'Content-Type'  => 'application/json',
                'Accept'        => 'application/json',
            ],
        ]);
    }

    /**
     * Publish an event to a single channel.
     *
     * @throws RelayException
     */
    public function publish(string $channel, string $event, mixed $data, ?string $excludeSocketId = null): void
    {
        $payload = [
            'channel' => $channel,
            'event'   => $event,
            'data'    => is_string($data) ? $data : json_encode($data),
        ];

        if ($excludeSocketId) {
            $payload['socket_id'] = $excludeSocketId;
        }

        $this->post('/events', $payload);
    }

    /**
     * Publish the same event to multiple channels at once.
     *
     * @param  string[]  $channels
     * @throws RelayException
     */
    public function publishToMultiple(array $channels, string $event, mixed $data): void
    {
        $encodedData = is_string($data) ? $data : json_encode($data);

        $batch = array_map(fn ($channel) => [
            'channel' => $channel,
            'event'   => $event,
            'data'    => $encodedData,
        ], $channels);

        $this->post('/events/batch', ['batch' => $batch]);
    }

    /**
     * Get information about a channel.
     *
     * @throws RelayException
     */
    public function getChannel(string $channel): array
    {
        return $this->get("/channels/{$channel}");
    }

    /**
     * Get all active channels.
     *
     * @throws RelayException
     */
    public function getChannels(): array
    {
        return $this->get('/channels');
    }

    /**
     * Get the members of a presence channel.
     *
     * @throws RelayException
     */
    public function getPresenceUsers(string $channel): array
    {
        return $this->get("/channels/{$channel}/users");
    }

    /**
     * Generate an authentication signature for a private or presence channel.
     * This is used by the auth endpoint in your Laravel app.
     */
    public function authenticate(string $socketId, string $channelName, ?array $channelData = null): array
    {
        $stringToSign = "{$socketId}:{$channelName}";

        if ($channelData !== null) {
            $channelDataJson = json_encode($channelData);
            $stringToSign .= ":{$channelDataJson}";
        }

        $signature = hash_hmac('sha256', $stringToSign, $this->config['secret']);
        $auth = $this->config['key'] . ':' . $signature;

        $result = ['auth' => $auth];

        if ($channelData !== null) {
            $result['channel_data'] = json_encode($channelData);
        }

        return $result;
    }

    // ─── HTTP Helpers ─────────────────────────────────────────────────

    private function post(string $path, array $body): array
    {
        try {
            $response = $this->http->post(
                $this->appPath($path),
                ['json' => $body]
            );

            return json_decode($response->getBody()->getContents(), true) ?? [];
        } catch (GuzzleException $e) {
            throw new RelayException("Relay publish failed: {$e->getMessage()}", 0, $e);
        }
    }

    private function get(string $path): array
    {
        try {
            $response = $this->http->get($this->appPath($path));
            return json_decode($response->getBody()->getContents(), true) ?? [];
        } catch (GuzzleException $e) {
            throw new RelayException("Relay request failed: {$e->getMessage()}", 0, $e);
        }
    }

    private function appPath(string $path): string
    {
        return '/apps/' . $this->config['app_id'] . $path;
    }

    private function buildBaseUrl(): string
    {
        $scheme = ($this->config['tls'] ?? false) ? 'https' : 'http';
        $host   = $this->config['host'] ?? '127.0.0.1';
        $port   = $this->config['port'] ?? 6001;

        return "{$scheme}://{$host}:{$port}";
    }
}
