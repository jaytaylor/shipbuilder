/**
 * Main app.
 */
var sb = angular.module('sb', ['ajoslin.promise-tracker', 'ui', 'ui.bootstrap', 'sbServices', '$strap.directives']).
    config(['$routeProvider', '$locationProvider', '$httpProvider', function($routeProvider, $locationProvider, $httpProvider) {
        //$dialogProvider.options({  });
        $locationProvider.html5Mode(true);

        $routeProvider.
            when('/web/login', { templateUrl: '/web/partials/login.html', pageKey: 'login' }).
            when('/web/', { templateUrl: '/web/partials/app-list.html', pageKey: 'app', controller: AppListController }).
            when('/web/apps', { templateUrl: '/web/partials/app-list.html', pageKey: 'app', controller: AppListController }).
            otherwise({ redirectTo: '/web' });
    }]).
    run(function ($rootScope, $http, $route) {
        $rootScope.$on('$routeChangeSuccess', function(angularEvent, currentRoute, previousRoute) {
            $('.pagekey').toggleClass('active', false);
            $('.pagekey_' + currentRoute.pageKey).toggleClass('active', true);
        });
    });

/**
 * Modal input field automatic focuser.
 */
sb.directive('focusInput', ['$timeout', function($timeout) {
    return {
        restrict: 'A',
        link: function(scope, element, attrs) {
            element.bind('click', function() {
                $timeout(function() {
                    $(attrs.focusInput)[0].focus();
                });
            });
        }
    };
}]);

/**
 * Get a property by nested name.
 */
var byString = function(o, s) {
    //console.log('o=' + o + ', s=' + s);
    s = s.replace(/\[(\w+)\]/g, '.$1'); // convert indexes to properties
    s = s.replace(/^\./, '');           // strip a leading dot
    var a = s.split('.');
    while (a.length) {
        var n = a.shift();
        if (n in o) {
            o = o[n];
        } else {
            return;
        }
    }
    return o;
};

