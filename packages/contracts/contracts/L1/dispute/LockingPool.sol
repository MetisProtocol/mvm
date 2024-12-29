// SPDX-License-Identifier: MIT
pragma solidity 0.8.15;

import {OwnableUpgradeable} from "@openzeppelin/contracts-upgradeable/access/OwnableUpgradeable.sol";
import {MetisConfig} from "../config/MetisConfig.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import {IDisputeGameFactory} from "../dispute/interfaces/IDisputeGameFactory.sol";
import {IFaultDisputeGame} from "../dispute/interfaces/IFaultDisputeGame.sol";
import {IDisputeGame} from "../dispute/interfaces/IDisputeGame.sol";
import "contracts/L1/dispute/lib/Types.sol";
import "contracts/L1/dispute/lib/Errors.sol";
import {ILockingPool} from "./interfaces/ILockingPool.sol";

/// @title LockingPool
/// @notice A locking pool contract that allows users to lock tokens with a delayed withdrawal mechanism
/// @dev Based on DelayedWETH's delayed withdrawal mechanism
contract LockingPool is OwnableUpgradeable, ILockingPool {
    using SafeERC20 for IERC20;

    /// @notice Struct representing a withdrawal request
    struct WithdrawalRequest {
        uint256 amount;
        uint256 timestamp;
    }

    /// @notice The token being locked in the pool
    IERC20 public token;

    /// @notice The withdrawal delay period in seconds
    uint256 public lockPeriod;

    /// @notice The slash ratio (base 10000, i.e., 100.00%)
    uint256 public slashRatio;

    /// @notice The MetisConfig contract
    MetisConfig public config;

    /// @notice The DisputeGameFactory contract
    IDisputeGameFactory public disputeGameFactory;

    /// @notice Mapping of user withdrawal requests
    mapping(address => WithdrawalRequest) public withdrawals;

    /// @notice Mapping of user balances
    mapping(address => uint256) public balanceOf;

    /// @notice Total amount of tokens locked
    uint256 public totalLocked;

    /// @notice Emitted when tokens are deposited
    event Deposit(address indexed user, uint256 amount);

    /// @notice Emitted when a withdrawal is requested
    event Unlock(address indexed user, uint256 amount);

    /// @notice Emitted when tokens are withdrawn
    event Withdraw(address indexed user, uint256 amount);

    /// @notice Emitted when tokens are slashed
    event Slashed(address indexed recipient, uint256 amount);

    /// @notice Emitted when slash ratio is updated
    event SlashRatioUpdated(uint256 oldRatio, uint256 newRatio);

    /// @notice Emitted when lock period is updated
    event LockPeriodUpdated(uint256 oldPeriod, uint256 newPeriod);

    /// @param _token The ERC20 token contract address
    /// @param _lockPeriod Initial lock period in seconds
    /// @param _slashRatio Initial slash ratio (base 10000)
    /// @param _disputeGameFactory The DisputeGameFactory contract address
    constructor(
        address _token,
        uint256 _lockPeriod,
        uint256 _slashRatio,
        IDisputeGameFactory _disputeGameFactory
    ) {
        require(_token != address(0), "LockingPool: invalid token");
        require(address(_disputeGameFactory) != address(0), "LockingPool: invalid factory");
        token = IERC20(_token);
        lockPeriod = _lockPeriod;
        slashRatio = _slashRatio;
        disputeGameFactory = _disputeGameFactory;
        initialize(address(0), MetisConfig(address(0)));
    }

    /// @notice Initializes the contract
    /// @param _owner The owner address
    /// @param _config The MetisConfig contract address
    function initialize(address _owner, MetisConfig _config) public initializer {
        __Ownable_init();
        _transferOwnership(_owner);
        config = _config;
    }

    /// @notice Deposits tokens into the pool
    /// @param _amount Amount of tokens to deposit
    function deposit(uint256 _amount) external {
        require(_amount > 0, "LockingPool: zero deposit");
        
        token.safeTransferFrom(msg.sender, address(this), _amount);
        balanceOf[msg.sender] += _amount;
        totalLocked += _amount;
        
        emit Deposit(msg.sender, _amount);
    }

    /// @notice Requests to unlock tokens
    /// @param _amount Amount of tokens to unlock
    function unlock(uint256 _amount) external {
        require(balanceOf[msg.sender] >= _amount, "LockingPool: insufficient balance");
        
        WithdrawalRequest storage wd = withdrawals[msg.sender];
        wd.timestamp = block.timestamp;
        wd.amount += _amount;
        
        emit Unlock(msg.sender, _amount);
    }

    /// @notice Withdraws unlocked tokens
    /// @param _amount Amount of tokens to withdraw
    function withdraw(uint256 _amount) external {
        require(!config.paused(), "LockingPool: contract is paused");
        
        WithdrawalRequest storage wd = withdrawals[msg.sender];
        require(wd.amount >= _amount, "LockingPool: insufficient unlocked withdrawal");
        require(wd.timestamp > 0, "LockingPool: withdrawal not unlocked");
        require(
            wd.timestamp + lockPeriod <= block.timestamp,
            "LockingPool: withdrawal delay not met"
        );
        
        wd.amount -= _amount;
        balanceOf[msg.sender] -= _amount;
        totalLocked -= _amount;
        
        token.safeTransfer(msg.sender, _amount);
        emit Withdraw(msg.sender, _amount);
    }

    /// @notice Slashes a percentage of tokens from the pool
    /// @param _recipient Address to receive the slashed tokens
    function slash(address _recipient) external {
        require(_recipient != address(0), "LockingPool: invalid recipient");
        require(totalLocked > 0, "LockingPool: no tokens to slash");

        // Verify the caller is a valid dispute game
        IFaultDisputeGame game = IFaultDisputeGame(msg.sender);
        (GameType gameType, Claim rootClaim, bytes memory extraData) = game.gameData();

        // Get the verified address of the game based on the game data
        (IDisputeGame factoryRegisteredGame,) = disputeGameFactory.games({
            _gameType: gameType,
            _rootClaim: rootClaim,
            _extraData: extraData
        });

        // Must be a valid game
        if (address(factoryRegisteredGame) != address(game)) revert UnregisteredGame();

        // Must be a game that resolved in favor of the challenger
        if (game.status() != GameStatus.CHALLENGER_WINS) {
            revert InvalidGameStatus();
        }

        // Track actual slashed amount
        uint256 actualSlashedAmount;
        
        // Cache users array to avoid multiple storage reads
        address[] memory users = getUsers();
        uint256 usersLength = users.length;
        
        // First pass: calculate actual slashed amount
        for (uint256 i; i < usersLength;) {
            address user = users[i];
            uint256 userBalance = balanceOf[user];
            if (userBalance > 0) {
                // Calculate user's slash amount
                uint256 userSlashAmount = (userBalance * slashRatio) / 10000;
                if (userSlashAmount > 0) {
                    actualSlashedAmount += userSlashAmount;
                    // Update user balance
                    balanceOf[user] = userBalance - userSlashAmount;
                }
            }
            // Gas optimization for loops
            unchecked { ++i; }
        }

        // Require some tokens were actually slashed
        require(actualSlashedAmount > 0, "LockingPool: slash amount too small");

        // Update total locked amount
        totalLocked -= actualSlashedAmount;

        // Transfer the actual slashed amount
        token.safeTransfer(_recipient, actualSlashedAmount);
        emit Slashed(_recipient, actualSlashedAmount);
    }

    /// @notice Updates the slash ratio
    /// @param _newRatio New slash ratio (base 10000)
    function setSlashRatio(uint256 _newRatio) external onlyOwner {
        uint256 oldRatio = slashRatio;
        slashRatio = _newRatio;
        emit SlashRatioUpdated(oldRatio, _newRatio);
    }

    /// @notice Updates the lock period
    /// @param _newPeriod New lock period in seconds
    function setLockPeriod(uint256 _newPeriod) external onlyOwner {
        uint256 oldPeriod = lockPeriod;
        lockPeriod = _newPeriod;
        emit LockPeriodUpdated(oldPeriod, _newPeriod);
    }

    /// @notice Gets the withdrawal request details for a user
    /// @param _user User address
    /// @return amount Amount requested for withdrawal
    /// @return timestamp Timestamp of the withdrawal request
    function getWithdrawalRequest(address _user) 
        external 
        view 
        returns (uint256 amount, uint256 timestamp) 
    {
        WithdrawalRequest memory wd = withdrawals[_user];
        return (wd.amount, wd.timestamp);
    }

    /// @notice Gets all users with non-zero balances
    /// @return users Array of user addresses with non-zero balances
    function getUsers() public view returns (address[] memory users) {
        // First pass: count users with balance
        uint256 count;
        uint256 maxIterations = 1000; // Arbitrary limit for gas optimization
        
        for (uint160 i = 1; i <= maxIterations;) {
            if (balanceOf[address(i)] > 0) {
                unchecked { ++count; }
            }
            unchecked { ++i; }
        }

        // Second pass: populate array
        users = new address[](count);
        uint256 index;
        
        for (uint160 i = 1; i <= maxIterations && index < count;) {
            address user = address(i);
            if (balanceOf[user] > 0) {
                users[index] = user;
                unchecked { ++index; }
            }
            unchecked { ++i; }
        }
    }
} 