// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;
/* Contract Imports */
/* External Imports */
import { Ownable } from "@openzeppelin/contracts/access/Ownable.sol";
import { IERC20 } from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import { iMVM_DiscountOracle } from "./iMVM_DiscountOracle.sol";
import { Lib_AddressResolver } from "../libraries/resolver/Lib_AddressResolver.sol";
import { MVM_AddressManager } from "../libraries/resolver/MVM_AddressManager.sol";
contract MVM_FeeCollector is Ownable, Lib_AddressResolver {
    event FeeCollected(address collector, address to, uint256 amount);

    // Current l2 gas price
    struct FeeScheme{
       uint8   pctVerifier;
    }
   
    
    address[] public verifiers;
    
    
    constructor(
      address _addressManager,
      address _metis
    )
      Ownable() 
      Lib_AddressResolver(_addressManager)
    {
      
    }
    
}
