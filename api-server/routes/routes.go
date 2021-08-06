package routes

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/bentoml/yatai/api-server/config"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"

	"github.com/bentoml/yatai/api-server/controllers/controllersv1"
	"github.com/bentoml/yatai/api-server/models"
	"github.com/bentoml/yatai/api-server/services"
	"github.com/bentoml/yatai/common/consts"
	"github.com/bentoml/yatai/common/scookie"
	"github.com/bentoml/yatai/common/yataicontext"
	"github.com/bentoml/yatai/schemas/schemasv1"
	"github.com/gin-gonic/gin"
	"github.com/loopfz/gadgeto/tonic"
	"github.com/pkg/errors"
	"github.com/wI2L/fizz"
	"github.com/wI2L/fizz/openapi"
)

var pwd, _ = os.Getwd()

var staticDirs = map[string]string{
	"/swagger": path.Join(pwd, "statics/swagger-ui"),
}

func NewRouter() (*fizz.Fizz, error) {
	engine := gin.New()

	store := cookie.NewStore([]byte(config.YataiConfig.Server.SessionSecretKey))
	engine.Use(sessions.Sessions("yatai-session", store))

	oauthGrp := engine.Group("oauth")
	oauthGrp.GET("/github", controllersv1.GithubOAuthLogin)

	callbackGrp := engine.Group("callback")
	callbackGrp.GET("/github", controllersv1.GithubOAuthCallBack)

	fizzApp := fizz.NewFromEngine(engine)

	// Override type names.
	// fizz.Generator().OverrideTypeName(reflect.TypeOf(Fruit{}), "SweetFruit")

	// Initialize the informations of
	// the API that will be served with
	// the specification.
	infos := &openapi.Info{
		Title:       "yatai api server",
		Description: `This is yatai api server.`,
		Version:     "1.0.0",
	}
	// Create a new route that serve the OpenAPI spec.
	fizzApp.GET("/openapi.json", nil, fizzApp.OpenAPI(infos, "json"))

	rootGrp := fizzApp.Group("/api/v1", "api v1", "api v1")

	// Setup routes.
	authRoutes(rootGrp)
	userRoutes(rootGrp)
	organizationRoutes(rootGrp)

	if len(fizzApp.Errors()) != 0 {
		return nil, fmt.Errorf("fizz errors: %v", fizzApp.Errors())
	}

	for p, root := range staticDirs {
		engine.Static(p, root)
	}

	engine.NoRoute(func(ctx *gin.Context) {
		if strings.HasPrefix(ctx.Request.URL.Path, "/api/") {
			ctx.JSON(http.StatusNotFound, &schemasv1.MsgSchema{Message: fmt.Sprintf("not found this router with method %s", ctx.Request.Method)})
			return
		}

		for p := range staticDirs {
			if strings.HasPrefix(ctx.Request.URL.Path, p) {
				ctx.JSON(http.StatusNotFound, &schemasv1.MsgSchema{Message: fmt.Sprintf("not found this router with method %s", ctx.Request.Method)})
				return
			}
		}
	})

	return fizzApp, nil
}

func getLoginUser(ctx *gin.Context) (user *models.User, err error) {
	apiToken := ctx.GetHeader(consts.YataiApiTokenHeaderName)

	// nolint: gocritic
	if apiToken != "" {
		user, err = services.UserService.GetByApiToken(ctx, apiToken)
		if err != nil {
			err = errors.Wrap(err, "get user by api token")
			return
		}
	} else {
		username := scookie.GetUsernameFromCookie(ctx)
		if username == "" {
			err = errors.New("username in cookie is empty")
			return
		}
		user, err = services.UserService.GetByName(ctx, username)
		if err != nil {
			err = errors.Wrapf(err, "get user by name in cookie %s", username)
			return
		}
	}

	yataicontext.SetUserName(ctx, user.Name)
	services.SetLoginUser(ctx, user)
	return
}

func requireLogin(ctx *gin.Context) {
	_, loginErr := getLoginUser(ctx)
	if loginErr != nil {
		msg := schemasv1.MsgSchema{Message: loginErr.Error()}
		ctx.AbortWithStatusJSON(http.StatusForbidden, &msg)
		return
	}
	ctx.Next()
}

func authRoutes(grp *fizz.RouterGroup) {
	grp = grp.Group("/auth", "auth", "auth")

	grp.POST("/register", []fizz.OperationOption{
		fizz.ID("Register an user"),
		fizz.Summary("Register an user"),
	}, tonic.Handler(controllersv1.AuthController.Register, 200))

	grp.POST("/login", []fizz.OperationOption{
		fizz.ID("Login an user"),
		fizz.Summary("Login an user"),
	}, tonic.Handler(controllersv1.AuthController.Login, 200))

	grp.GET("/current", []fizz.OperationOption{
		fizz.ID("Get current user"),
		fizz.Summary("Get current user"),
	}, requireLogin, tonic.Handler(controllersv1.AuthController.GetCurrentUser, 200))
}

func userRoutes(grp *fizz.RouterGroup) {
	grp = grp.Group("/users", "users", "users api")

	resourceGrp := grp.Group("/:userName", "user resource", "user resource")

	resourceGrp.GET("", []fizz.OperationOption{
		fizz.ID("Get an user"),
		fizz.Summary("Get an user"),
	}, requireLogin, tonic.Handler(controllersv1.UserController.Get, 200))

	grp.GET("", []fizz.OperationOption{
		fizz.ID("List users"),
		fizz.Summary("List users"),
	}, requireLogin, tonic.Handler(controllersv1.UserController.List, 200))
}

func organizationRoutes(grp *fizz.RouterGroup) {
	grp = grp.Group("/orgs", "organizations", "organizations")

	resourceGrp := grp.Group("/:orgName", "organization resource", "organization resource")

	resourceGrp.GET("", []fizz.OperationOption{
		fizz.ID("Get an organization"),
		fizz.Summary("Get an organization"),
	}, requireLogin, tonic.Handler(controllersv1.OrganizationController.Get, 200))

	resourceGrp.PATCH("", []fizz.OperationOption{
		fizz.ID("Update an organization"),
		fizz.Summary("Update an organization"),
	}, requireLogin, tonic.Handler(controllersv1.OrganizationController.Update, 200))

	resourceGrp.GET("/members", []fizz.OperationOption{
		fizz.ID("List organization members"),
		fizz.Summary("Get organization members"),
	}, requireLogin, tonic.Handler(controllersv1.OrganizationMemberController.List, 200))

	resourceGrp.POST("/members", []fizz.OperationOption{
		fizz.ID("Create an organization member"),
		fizz.Summary("Create an organization member"),
	}, requireLogin, tonic.Handler(controllersv1.OrganizationMemberController.Create, 200))

	resourceGrp.DELETE("/members", []fizz.OperationOption{
		fizz.ID("Remove an organization member"),
		fizz.Summary("Remove an organization member"),
	}, requireLogin, tonic.Handler(controllersv1.OrganizationMemberController.Delete, 200))

	grp.GET("", []fizz.OperationOption{
		fizz.ID("List organizations"),
		fizz.Summary("List organizations"),
	}, requireLogin, tonic.Handler(controllersv1.OrganizationController.List, 200))

	grp.POST("", []fizz.OperationOption{
		fizz.ID("Create organization"),
		fizz.Summary("Create organization"),
	}, requireLogin, tonic.Handler(controllersv1.OrganizationController.Create, 200))

	clusterRoutes(resourceGrp)
}

func clusterRoutes(grp *fizz.RouterGroup) {
	grp = grp.Group("/clusters", "clusters", "clusters")

	resourceGrp := grp.Group("/:clusterName", "cluster resource", "cluster resource")

	resourceGrp.GET("", []fizz.OperationOption{
		fizz.ID("Get a cluster"),
		fizz.Summary("Get a cluster"),
	}, requireLogin, tonic.Handler(controllersv1.ClusterController.Get, 200))

	resourceGrp.PATCH("", []fizz.OperationOption{
		fizz.ID("Update a cluster"),
		fizz.Summary("Update a cluster"),
	}, requireLogin, tonic.Handler(controllersv1.ClusterController.Update, 200))

	resourceGrp.GET("/members", []fizz.OperationOption{
		fizz.ID("List cluster members"),
		fizz.Summary("List cluster members"),
	}, requireLogin, tonic.Handler(controllersv1.ClusterMemberController.List, 200))

	resourceGrp.POST("/members", []fizz.OperationOption{
		fizz.ID("Create a cluster member"),
		fizz.Summary("Create a cluster member"),
	}, requireLogin, tonic.Handler(controllersv1.ClusterMemberController.Create, 200))

	resourceGrp.DELETE("/members", []fizz.OperationOption{
		fizz.ID("Remove a cluster member"),
		fizz.Summary("Remove a cluster member"),
	}, requireLogin, tonic.Handler(controllersv1.ClusterMemberController.Delete, 200))

	grp.GET("", []fizz.OperationOption{
		fizz.ID("List clusters"),
		fizz.Summary("List clusters"),
	}, requireLogin, tonic.Handler(controllersv1.ClusterController.List, 200))

	grp.POST("", []fizz.OperationOption{
		fizz.ID("Create cluster"),
		fizz.Summary("Create cluster"),
	}, requireLogin, tonic.Handler(controllersv1.ClusterController.Create, 200))

	bundleRoutes(resourceGrp)
}

func bundleRoutes(grp *fizz.RouterGroup) {
	grp = grp.Group("/bundles", "bundles", "bundles")

	resourceGrp := grp.Group("/:bundleName", "bundle resource", "bundle resource")

	resourceGrp.GET("", []fizz.OperationOption{
		fizz.ID("Get a bundle"),
		fizz.Summary("Get a bundle"),
	}, requireLogin, tonic.Handler(controllersv1.BundleController.Get, 200))

	resourceGrp.PATCH("", []fizz.OperationOption{
		fizz.ID("Update a bundle"),
		fizz.Summary("Update a bundle"),
	}, requireLogin, tonic.Handler(controllersv1.BundleController.Update, 200))

	grp.GET("", []fizz.OperationOption{
		fizz.ID("List bundles"),
		fizz.Summary("List bundles"),
	}, requireLogin, tonic.Handler(controllersv1.BundleController.List, 200))

	grp.POST("", []fizz.OperationOption{
		fizz.ID("Create bundle"),
		fizz.Summary("Create bundle"),
	}, requireLogin, tonic.Handler(controllersv1.BundleController.Create, 200))

	bundleVersionRoutes(resourceGrp)
}

func bundleVersionRoutes(grp *fizz.RouterGroup) {
	grp = grp.Group("/versions", "bundle versions", "bundle versions")

	resourceGrp := grp.Group("/:version", "bundle version resource", "bundle version resource")

	resourceGrp.GET("", []fizz.OperationOption{
		fizz.ID("Get a bundle version"),
		fizz.Summary("Get a bundle version"),
	}, requireLogin, tonic.Handler(controllersv1.BundleVersionController.Get, 200))

	resourceGrp.PATCH("/start_upload", []fizz.OperationOption{
		fizz.ID("Start upload a bundle version"),
		fizz.Summary("Start upload a bundle version"),
	}, requireLogin, tonic.Handler(controllersv1.BundleVersionController.StartUpload, 200))

	resourceGrp.PATCH("/finish_upload", []fizz.OperationOption{
		fizz.ID("Finish upload a bundle version"),
		fizz.Summary("Finish upload a bundle version"),
	}, requireLogin, tonic.Handler(controllersv1.BundleVersionController.FinishUpload, 200))

	grp.GET("", []fizz.OperationOption{
		fizz.ID("List bundle versions"),
		fizz.Summary("List bundle versions"),
	}, requireLogin, tonic.Handler(controllersv1.BundleVersionController.List, 200))

	grp.POST("", []fizz.OperationOption{
		fizz.ID("Create bundle version"),
		fizz.Summary("Create bundle version"),
	}, requireLogin, tonic.Handler(controllersv1.BundleVersionController.Create, 200))
}