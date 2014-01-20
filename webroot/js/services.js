var sbModule = angular.module('sbServices', ['ngResource']);

sbModule.factory('App', function($resource) {
    return $resource('/api/v1/app/:name', { q: '' }, {
        get: { method: 'GET' }, //isArray: false },
        query: { method: 'GET'} //, params: { q: '' }//, isArray: false }
    });
});

